"""Synthetic Hackbot transport + native PDF + encrypted backup + isolated restore; no paid model."""
import subprocess,sys,base64,secrets,hashlib, http.cookiejar, json, os, ssl, time, types, urllib.request, uuid
from pathlib import Path
from backup import backup,restore

def client_for(args,compose):
    config=json.loads((args.state/'operator.json').read_text())
    if config['site']!='localhost' or config['bind']!='127.0.0.1':raise RuntimeError('Smoke only supports local HTTPS')
    cert=compose(args,'exec','-T','caddy','cat','/data/caddy/pki/authorities/local/root.crt',capture=True).stdout
    client=urllib.request.build_opener(urllib.request.HTTPSHandler(context=ssl.create_default_context(cadata=cert)),urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    base='https://localhost:'+config['port']
    credential=json.loads((args.credentials_file or args.state/'initial-admin.json').read_text())
    def request(method,path,data=None,headers=None,binary=False):
        req=urllib.request.Request(base+'/api/v1'+path,method=method,data=json.dumps(data).encode() if data is not None else None,headers={'Content-Type':'application/json','X-Requested-With':'orchis-web',**(headers or {})})
        with client.open(req,timeout=30) as response:
            value=response.read()
            return value if binary else json.loads(value) if value else None
    request('POST','/auth/login',credential)
    return request

def wait(fn,timeout=180):
    until=time.monotonic()+timeout
    while time.monotonic()<until:
        value=fn()
        if value:return value
        time.sleep(1)
    raise RuntimeError('Production smoke timed out')

def smoke(args,compose,init,up,probe):
    if args.state.exists():raise RuntimeError('Smoke requires a fresh state directory; existing installations are never overwritten')
    init(args)
    compose(args,'build','server','renderer')
    subprocess.run([sys.executable,str(Path(__file__).resolve().parents[1]/'ci/check-mitre-image.py'),
                    '--image','orchis-server:'+os.environ.get('ORCHIS_VERSION','local')],check=True,timeout=240)
    subprocess.run([sys.executable,str(Path(__file__).resolve().parents[1]/'hackbot/build.py')],check=True)
    compose(args,'build','nmap','hackbot')
    up(args);probe(args);api=client_for(args,compose)
    org=api('POST','/organizations',{'name':'LOCAL PRODUCTION SMOKE FIXTURE','slug':'local-smoke-'+uuid.uuid4().hex[:8],'description':'Synthetic integration fixture; no pentest or target requests are performed.','industry':'Local fixture'})
    addresses=[{'kind':'url','value':'http://fixture:8081'}]
    assets=[]
    for name,authorized,values in [('LOCAL FIXTURE TESTED',True,addresses),('LOCAL FIXTURE NOT AUTHORIZED',False,addresses),('LOCAL FIXTURE NO ADDRESS',True,[])]:
        assets.append(api('POST','/assets',{'category':'host','name':name,'testing_authorized':authorized,'is_internet_facing':True,'addresses':values}))
    api('PUT','/organizations/'+org['id']+'/assets',{'assets':[{'id':a['id'],'version':a['version']} for a in assets]})
    payload={'organization_id':org['id'],'asset_ids':[assets[0]['id']]}
    key=str(uuid.uuid4());assessment=api('POST','/scans',payload,{'Idempotency-Key':key})
    repeated=api('POST','/scans',payload,{'Idempotency-Key':key})
    if repeated['id']!=assessment['id']:raise RuntimeError('Launch idempotency failed')
    def finished():
        value=api('GET','/scans/'+assessment['id'])
        return value if value['status']=='incomplete' else None
    completed=wait(finished,timeout=1200)
    if len(completed['jobs'])!=3 or next(j for j in completed['jobs'] if j['tool']=='hackbot')['disposition']!='limited':raise RuntimeError('Synthetic coverage was not marked incomplete')
    if len(completed.get('summary','').split())>300:raise RuntimeError('Summary exceeds 300 words')
    def report_ready():
        page=api('GET','/reports?assessment_id='+assessment['id'])
        ready=[r for r in page['items'] if r['status']=='ready']
        if any(r['status']=='failed' for r in page['items']):raise RuntimeError('Production PDF render failed')
        return ready[0] if len(ready)==1 else None
    report=wait(report_ready);full=api('GET','/reports/'+report['id'])
    snapshot=api('GET','/reports/'+report['id']+'/export/json',binary=True)
    pdf=api('GET','/reports/'+report['id']+'/export/pdf',binary=True)
    if not pdf.startswith(b'%PDF-'):raise RuntimeError('Stored report PDF is invalid')
    tested=full['snapshot']['assets']
    if len(tested)!=1 or tested[0]['status']!='incomplete':raise RuntimeError('Synthetic transport fixture must not claim tested coverage or findings')
    if {m['action'] for m in tested[0].get('methods',[])}!={'nmap.tcp_connect','nuclei.scan','hackbot.pentest'}:raise RuntimeError('Hackbot method missing')
    # Store only fake, disabled provider credentials. Rotation must not call a provider.
    current=api('GET','/organizations/'+org['id']+'/ai')
    policy={k:current[k] for k in ['version','enabled','planning','remediation','planning_model','remediation_model','max_input_bytes','max_output_tokens','daily_tokens','daily_calls','daily_cost_micros','input_rate_micros','output_rate_micros']}
    policy.update(enabled=False,planning=False,remediation=False,consent=False,deepseek_key='LOCAL_FAKE_DEEPSEEK_NEVER_CALL',openai_key='LOCAL_FAKE_OPENAI_NEVER_CALL')
    api('PUT','/organizations/'+org['id']+'/ai',policy)
    from ops import private
    from rekey import rekey
    new_key=args.state/'fixture-new-ai.key';private(new_key,base64.b64encode(secrets.token_bytes(32)).decode())
    rotation=types.SimpleNamespace(**vars(args));rotation.key_file=new_key
    if rekey(rotation,compose)!=1:raise RuntimeError('Expected one fake AI policy to be reencrypted')
    archive=backup(args,compose)
    restored=types.SimpleNamespace(**vars(args));restored.command='restore';restored.state=args.state.with_name(args.state.name+'-restored');restored.project=args.project+'-restored';restored.port=args.port+1;restored.archive=archive;restored.key_file=args.state/'backup.key';restored.credentials_file=None
    restore(restored,compose,init,up,probe)
    after=client_for(restored,compose)
    if after('GET','/reports/'+report['id']+'/export/json',binary=True)!=snapshot:raise RuntimeError('Frozen snapshot changed after restore')
    if hashlib.sha256(after('GET','/reports/'+report['id']+'/export/pdf',binary=True)).digest()!=hashlib.sha256(pdf).digest():raise RuntimeError('Stored PDF changed after restore')
    if not after('GET','/organizations/'+org['id']+'/ai')['has_keys']:raise RuntimeError('Restored fake AI credentials missing')
    key_check=json.loads(compose(restored,'run','--rm','--no-deps','-e','ORCHIS_AI_REKEY_NEW_FILE=/run/secrets/ai-master.key','server','ai-rekey',capture=True).stdout)
    if key_check['reencrypted_policies']!=0:raise RuntimeError('Restored master key could not decrypt credentials idempotently')
    result={'ai_rekey':'verified','assessment_id':assessment['id'],'report_id':report['id'],'tested_assets':0,'incomplete_assets':1,'engine':'hackbot','synthetic_transport_fixture':True,'snapshot_sha256':hashlib.sha256(snapshot).hexdigest(),'pdf_sha256':hashlib.sha256(pdf).hexdigest(),'source_project':args.project,'restored_project':restored.project,'encrypted_backup':str(archive),'restore':'verified'}
    output=args.state/'smoke-result.json';output.write_text(json.dumps(result,indent=2));os.chmod(output,0o600)
    print(json.dumps(result))
