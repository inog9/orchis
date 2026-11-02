#!/usr/bin/env python3
"""Bukti upgrade/rollback pada salinan DB lama; sumber hanya dibaca, dump dienkripsi, DB uji dibersihkan."""
import argparse,hashlib,http.cookiejar,json,os,re,secrets,shutil,subprocess,time,urllib.request
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
COMPOSE=['docker','compose','-f',str(ROOT/'deploy/compose.yml'),'exec','-T','pg']

def run(argv,**kw):
    return subprocess.run(argv,check=True,timeout=180,stderr=subprocess.PIPE,**kw)
def sql(database,query):
    return run(COMPOSE+['psql','-U','orchis','-d',database,'-At','-v','ON_ERROR_STOP=1','-c',query],stdout=subprocess.PIPE,text=True).stdout.strip()
def ident(value):return '"'+value.replace('"','""')+'"'
def fingerprints(database,columns,normalize=False):
    result={}
    for table,cols in columns.items():
        expressions=[]
        for col in cols:
            value='a.'+ident(col)
            if normalize and table=='assets' and col in ['testing_authorized','authorized_by','authorized_at']:
                replacement='false' if col=='testing_authorized' else 'NULL'
                value=f"CASE WHEN a.is_demo AND EXISTS(SELECT 1 FROM asset_addresses ad WHERE ad.tenant_id=a.tenant_id AND ad.asset_id=a.id AND ad.value<>'127.0.0.1') THEN {replacement} ELSE {value} END"
            expressions.append(value+' AS '+ident(col))
        query="SELECT json_build_object('count',count(*),'sha256',encode(sha256(convert_to(COALESCE(string_agg(digest,'' ORDER BY digest),''),'UTF8')),'hex')) FROM (SELECT encode(sha256(convert_to(row_to_json(r)::text,'UTF8')),'hex') digest FROM (SELECT "+','.join(expressions)+' FROM public.'+ident(table)+' a) r) x'
        result[table]=json.loads(sql(database,query))
    return result

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-db',required=True)
    parser.add_argument('--out',type=Path,required=True)
    parser.add_argument('--password-file',type=Path,required=True,help='existing admin password; read only inside this process')
    parser.add_argument('--port',type=int,default=18220)
    args=parser.parse_args()
    if not re.fullmatch('[a-zA-Z0-9_]+',args.source_db):raise RuntimeError('Invalid source database name')
    if args.out.exists():raise RuntimeError('Evidence directory must be new')
    if args.password_file.is_symlink() or args.password_file.stat().st_mode&0o077:raise RuntimeError('Admin password file must be private')
    args.out.mkdir(mode=0o700,parents=True)
    if int(sql(args.source_db,'SELECT pg_database_size(current_database())'))>512<<20:raise RuntimeError('Source exceeds this bounded fixture size')
    if shutil.disk_usage(args.out).free<2<<30:raise RuntimeError('Need 2 GiB free for the bounded upgrade fixture')
    key=args.out/'backup.key';key.write_text(secrets.token_hex(32));key.chmod(0o600)
    dump=args.out/'old.dump';encrypted=args.out/'old.dump.age';restored=args.out/'restored.dump'
    created=[];server=None;result={'source_database':args.source_db,'source_read_only':True}
    try:
        with dump.open('xb') as target:run(COMPOSE+['pg_dump','-U','orchis','-d',args.source_db,'-Fc'],stdout=target)
        dump.chmod(0o600)
        with dump.open('rb') as src,encrypted.open('xb') as target:run([str(ROOT/'bin/backup-seal'),'-key-file',str(key)],stdin=src,stdout=target)
        encrypted.chmod(0o600)
        with encrypted.open('rb') as src,restored.open('xb') as target:run([str(ROOT/'bin/backup-seal'),'-decrypt','-key-file',str(key)],stdin=src,stdout=target)
        restored.chmod(0o600)
        digest=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
        if digest(dump)!=digest(restored):raise RuntimeError('Encrypted backup did not restore identical dump bytes')
        result['dump_sha256']=digest(dump);result['encrypted_bytes']=encrypted.stat().st_size
        dbs=['orchis_t5_upgrade_'+secrets.token_hex(6),'orchis_t5_rollback_'+secrets.token_hex(6)]
        for database in dbs:
            sql('postgres','CREATE DATABASE '+ident(database));created.append(database)
            with restored.open('rb') as src:run(COMPOSE+['pg_restore','-U','orchis','-d',database,'--exit-on-error'],stdin=src,stdout=subprocess.DEVNULL)
        upgraded,rollback=dbs
        version=lambda database:int(sql(database,'SELECT max(version_id) FROM goose_db_version WHERE is_applied'))
        result['old_schema']=version(upgraded)
        columns=json.loads(sql(upgraded,"SELECT json_object_agg(table_name,cols) FROM (SELECT table_name,json_agg(column_name ORDER BY ordinal_position) cols FROM information_schema.columns WHERE table_schema='public' AND table_name<>'goose_db_version' GROUP BY table_name) t"))
        original=fingerprints(upgraded,columns)
        expected=fingerprints(upgraded,columns,normalize=True)
        result['expected_safety_changes']=[name for name in original if original[name]!=expected[name]]
        env={key:value for key,value in os.environ.items() if not key.startswith('ORCHIS_')}
        env.update(ORCHIS_ENV_FILE='',ORCHIS_MITRE_URL='off',ORCHIS_DATABASE_URL=f'postgres://orchis_app:orchis_app@127.0.0.1:15432/{upgraded}',ORCHIS_MIGRATE_URL=f'postgres://orchis:orchis@127.0.0.1:15432/{upgraded}')
        with (args.out/'migrate.log').open('w') as log:run([str(ROOT/'bin/server-t5-upgrade'),'migrate'],env=env,stdout=log)
        result['new_schema']=version(upgraded)
        actual=fingerprints(upgraded,columns)
        if actual!=expected:raise RuntimeError('Old-column data changed unexpectedly in: '+','.join(name for name in expected if expected[name]!=actual[name]))
        if fingerprints(rollback,columns)!=original or version(rollback)!=result['old_schema']:raise RuntimeError('Forward-only rollback restore did not preserve old schema/data')
        result['tables_verified']=len(original);result['counts']={name:row['count'] for name,row in original.items()}
        result['old_columns_preserved']=True;result['rollback_schema_and_data_preserved']=True
        # Login hanya pada salinan; kredensial tidak dicetak atau diberikan lewat argv.
        admin=json.loads(sql(upgraded,"SELECT json_build_object('email',u.email,'tenant',t.name) FROM users u JOIN roles r ON r.tenant_id=u.tenant_id AND r.id=u.role_id JOIN tenants t ON t.id=u.tenant_id WHERE r.key='admin' AND u.status='active' AND u.deleted_at IS NULL ORDER BY u.created_at LIMIT 1"))
        env.update(ORCHIS_HTTP_ADDR=f'127.0.0.1:{args.port}',ORCHIS_TENANT=admin['tenant'],ORCHIS_ADMIN_EMAIL=admin['email'],ORCHIS_ADMIN_PASSWORD=secrets.token_urlsafe(32),ORCHIS_WEB_DIR=str(ROOT/'web/dist'))
        log=(args.out/'server.log').open('w')
        server=subprocess.Popen([str(ROOT/'bin/server-t5-upgrade'),'serve'],env=env,stdout=log,stderr=log)
        base=f'http://127.0.0.1:{args.port}'
        for attempt in range(100):
            try:
                with urllib.request.urlopen(base+'/api/healthz',timeout=2) as response:
                    if response.status==200:break
            except OSError:time.sleep(.1)
        else:raise RuntimeError('Upgraded application was not healthy')
        opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        body=json.dumps({'email':admin['email'],'password':args.password_file.read_text().strip()}).encode()
        request=urllib.request.Request(base+'/api/v1/auth/login',data=body,headers={'Content-Type':'application/json','X-Requested-With':'orchis-web'})
        with opener.open(request,timeout=10) as response:assert response.status==200
        for path in ['/api/v1/home','/api/v1/organizations','/api/v1/assets','/api/v1/reports']:
            with opener.open(base+path,timeout=10) as response:assert response.status==200
        report=sql(upgraded,'SELECT id FROM reports ORDER BY created_at LIMIT 1')
        if report:
            with opener.open(base+'/api/v1/reports/'+report+'/export/json',timeout=10) as response:
                snapshot=json.load(response);assert response.status==200 and snapshot.get('organization')
        result['login_and_api_smoke']=True;result['existing_report_json']=bool(report)
        (args.out/'result.json').write_text(json.dumps(result,indent=2)+'\n')
        print(json.dumps({k:v for k,v in result.items() if k!='counts'},indent=2))
    finally:
        if server:
            server.terminate()
            try:server.wait(timeout=10)
            except subprocess.TimeoutExpired:server.kill();server.wait()
        for database in reversed(created):sql('postgres','DROP DATABASE '+ident(database)+' WITH (FORCE)')
        dump.unlink(missing_ok=True);restored.unlink(missing_ok=True)

if __name__=='__main__':main()
