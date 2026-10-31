"""Encrypted, consistent backups and isolated restores for one owned Compose project."""
import contextlib, hashlib, io, json, os, re, shutil, subprocess, tarfile, tempfile, threading, time, zipfile
from pathlib import Path, PurePosixPath

STATE_FILES={
 'postgres_password','owner_password','app_password','maintenance_password',
 's3_user','s3_password','rabbitmq.conf','ai-master.key','renderer_token',
 'migrate.json','provision.json','server.json','nmap.json','nuclei.json','ai.json',
 'initial-admin.json','release.key',
}
TABLES=['organizations','assets','agents','users','findings','assessments','reports']
LIMIT=8<<30

class BoundedWriter:
    def __init__(self,stream,limit):self.stream,self.limit,self.position=stream,limit,0
    def write(self,value):
        if self.position+len(value)>self.limit:raise RuntimeError('Encrypted backup input exceeds configured size limit')
        written=self.stream.write(value);self.position+=written;return written
    def tell(self):return self.position
    def seek(self,*args):raise io.UnsupportedOperation('streaming archive')
    def flush(self):self.stream.flush()

def run(argv,**kw):return subprocess.run(argv,check=True,**kw)
def cfg(args):return json.loads((args.state/'operator.json').read_text())
def container(args,compose,service):
    value=compose(args,'ps','-a','-q',service,capture=True).stdout.strip()
    if not re.fullmatch(r'[a-f0-9]{12,64}',value):raise RuntimeError('Expected one project container: '+service)
    item=json.loads(subprocess.check_output(['docker','inspect',value],text=True))[0]
    if item['Config']['Labels'].get('com.docker.compose.project')!=cfg(args)['project']:raise RuntimeError('Container ownership mismatch')
    return item

def wait_services(args,compose,services):
    deadline=time.monotonic()+90
    while time.monotonic()<deadline:
        ready=True
        for service in services:
            state=container(args,compose,service)['State']
            if not state.get('Running') or state.get('Health',{}).get('Status','healthy')!='healthy':ready=False
        if ready:return
        time.sleep(2)
    raise RuntimeError('Maintenance services did not recover')

def volume(args,compose,service,destination):
    item=container(args,compose,service)
    names=[m['Name'] for m in item['Mounts'] if m['Type']=='volume' and m['Destination']==destination]
    if len(names)!=1:raise RuntimeError('Expected one owned persistent volume')
    record=json.loads(subprocess.check_output(['docker','volume','inspect',names[0]],text=True))[0]
    if record.get('Labels',{}).get('com.docker.compose.project')!=cfg(args)['project']:raise RuntimeError('Volume ownership mismatch')
    return names[0]

def pg(args,compose,sql):
    result=compose(args,'exec','-T','pg','psql','-U','postgres','-d','orchis','-At','-v','ON_ERROR_STOP=1','-c',sql,capture=True)
    return result.stdout.strip()

def fingerprints(args,compose):
    counts={name:int(pg(args,compose,f'SELECT count(*) FROM {name}')) for name in TABLES}
    # Snapshot JSON and PDF keys must survive restoration byte-for-byte. Do not log contents.
    rows=pg(args,compose,"SELECT COALESCE(jsonb_agg(jsonb_build_array(id,snapshot,pdf_key) ORDER BY id),'[]') FROM reports")
    return {'counts':counts,'reports_sha256':hashlib.sha256(rows.encode()).hexdigest()}

def assert_idle(args,compose):
    # RabbitMQ is rebuilt on restore. Only terminal scan/render/provider work may
    # be snapshotted, so no published dispatch message can be lost with the broker.
    query="SELECT (SELECT count(*) FROM executions WHERE state IN ('ready','running'))+(SELECT count(*) FROM simulations WHERE status IN ('running','paused'))+(SELECT count(*) FROM assessments WHERE status='running')+(SELECT count(*) FROM reports WHERE status IN ('queued','rendering'))+(SELECT count(*) FROM ai_jobs WHERE status IN ('queued','running'))"
    if int(pg(args,compose,query)):
        raise RuntimeError('Wait for active scans, paused runs, report rendering and queued/running AI work to finish or cancel before backup')

def helper_image(args,compose):return container(args,compose,'server')['Config']['Image']
def seal_command(image,key,decrypt=False,project="orchis-ops"):
    argv=['docker','run','--rm','--label','io.orchis.ops-owner='+project,'-i','--network','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true','--memory','192m','--pids-limit','32','--cpus','1','--user','0:0','-v',str(key.resolve())+':/run/secrets/backup.key:ro','--entrypoint','/usr/local/bin/backup-seal',image,'-key-file','/run/secrets/backup.key']
    if decrypt:argv.append('-decrypt')
    return argv

def stream_member(archive,name,argv):
    # A watchdog bounds stalled Docker/DB commands; stderr never receives credentials in this module.
    with subprocess.Popen(argv,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL) as proc:
        timer=threading.Timer(180,proc.kill);timer.start()
        try:
            with archive.open(name,'w',force_zip64=True) as target:shutil.copyfileobj(proc.stdout,target,1<<20)
            if proc.wait()!=0:raise RuntimeError('Backup source command failed: '+name)
        finally:timer.cancel()

def volume_read(image,name,project):
    return ['docker','run','--rm','--label','io.orchis.ops-owner='+project,'--network','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true','--memory','128m','--pids-limit','32','--cpus','0.5','-v',name+':/source:ro','--entrypoint','/bin/tar',image,'-C','/source','-cf','-','.']

def record_status(args,success):
    # Non-secret operator status; directory mount keeps atomic replacements visible.
    folder=args.state/'ops-status'
    folder.mkdir(mode=0o755,exist_ok=True)
    usage=shutil.disk_usage(args.state)
    temporary=folder/('backup-'+os.urandom(4).hex()+'.partial')
    fd=os.open(temporary,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o644)
    try:
        with os.fdopen(fd,'w') as target:
            json.dump({'success':success,'timestamp':time.time(),'disk_free_bytes':usage.free,'disk_total_bytes':usage.total},target)
            target.flush();os.fsync(target.fileno())
        os.chmod(temporary,0o644);os.replace(temporary,folder/'backup.json')
    finally:temporary.unlink(missing_ok=True)

def backup(args,compose):
    try:result=_backup(args,compose)
    except BaseException:
        try:record_status(args,False)
        except OSError:pass # preserve original error even if the status disk is full
        raise
    record_status(args,True)
    return result

def _backup(args,compose):
    if (args.state/"ai-rekey.json").exists():raise RuntimeError("Resume AI key rotation before backup")
    state=cfg(args);key=args.key_file or args.state/'backup.key'
    if not key.is_file():raise RuntimeError('Missing separate backup key; run backup-key and store an out-of-band copy')
    info=key.stat()
    if info.st_mode&0o077 or info.st_size>128:raise RuntimeError('Backup key must be a private 0600 file')
    services=['server','nmap','nuclei','ai-worker','renderer','minio','caddy']
    running=[]
    for service in services:
        if container(args,compose,service)['State']['Running']:running.append(service)
    image=helper_image(args,compose)
    folder=args.state/'backups';folder.mkdir(mode=0o700,exist_ok=True)
    if shutil.disk_usage(folder).free<args.max_bytes+(1<<30):raise RuntimeError('Insufficient backup disk headroom for configured archive limit')
    filename='orchis-'+time.strftime('%Y%m%dT%H%M%SZ',time.gmtime())+'-'+os.urandom(3).hex()+'.zip.age'
    final=folder/filename;partial=folder/(filename+'.partial')
    try:
        # Stop writers first, then snapshot DB and filesystem. Never stop an unrelated project.
        writers=[s for s in running if s!='minio']
        if writers:compose(args,'stop','--timeout','30',*writers)
        assert_idle(args,compose)
        objects=json.loads(compose(args,'run','--rm','--no-deps','server','object-digests',capture=True).stdout)
        if 'minio' in running:compose(args,'stop','--timeout','30','minio')
        manifest={'format':1,'created_at':time.time(),'project':state['project'],'fingerprints':fingerprints(args,compose),'server_image':image,'pdf_sha256':objects,'smoke':bool(state.get('smoke'))}
        volumes={name:volume(args,compose,service,path) for name,service,path in [('s3','minio','/data'),('caddydata','caddy','/data'),('caddyconfig','caddy','/config')]}
        pg_id=container(args,compose,'pg')['Id'];pg_image=container(args,compose,'pg')['Config']['Image']
        fd=os.open(partial,os.O_CREAT|os.O_EXCL|os.O_WRONLY,0o600)
        with os.fdopen(fd,'wb') as output:
            with subprocess.Popen(seal_command(image,key,project=state['project']),stdin=subprocess.PIPE,stdout=output,stderr=subprocess.DEVNULL) as encrypt:
                try:
                    with zipfile.ZipFile(BoundedWriter(encrypt.stdin,args.max_bytes),'w',compression=zipfile.ZIP_DEFLATED,compresslevel=3,allowZip64=True) as archive:
                        archive.writestr('manifest.json',json.dumps(manifest))
                        dump=['docker','exec',pg_id,'sh','-c','set -eu; PGPASSWORD=$(cat /run/secrets/maintenance_password); export PGPASSWORD; export PGOPTIONS="-c statement_timeout=60000 -c lock_timeout=5000"; exec pg_dump -h 127.0.0.1 -U orchis_maintenance -d orchis --no-owner --no-acl -Fc']
                        stream_member(archive,'db.dump',dump)
                        for name,vid in volumes.items():stream_member(archive,'volumes/'+name+'.tar',volume_read(pg_image,vid,state['project']))
                        for name in sorted(STATE_FILES):
                            path=args.state/name
                            if path.is_symlink():raise RuntimeError('Secret symlink rejected')
                            if path.is_file():archive.write(path,'state/'+name)
                        for path in sorted((args.state/'agent-releases').rglob('*')):
                            if path.is_symlink():raise RuntimeError('Release symlink rejected')
                            if path.is_file():archive.write(path,'state/'+str(path.relative_to(args.state)))
                    encrypt.stdin.close()
                    if encrypt.wait(timeout=120)!=0:raise RuntimeError('Backup encryption failed')
                except BaseException:
                    encrypt.kill();encrypt.wait();raise
            output.flush();os.fsync(output.fileno())
        os.replace(partial,final)
        # Retention applies only to complete encrypted backups produced by this project.
        cutoff=time.time()-args.retention_days*86400
        for path in folder.glob('orchis-*.zip.age'):
            if path!=final and path.is_file() and not path.is_symlink() and path.stat().st_mtime<cutoff:path.unlink()
        print('Encrypted backup: '+str(final))
        print(json.dumps({'counts':manifest['fingerprints']['counts'],'archive_bytes':final.stat().st_size}))
        return final
    except BaseException:
        partial.unlink(missing_ok=True)
        raise
    finally:
        if running:
            compose(args,'start',*running)
            wait_services(args,compose,running)

def validate_tar(path,limit):
    total=0
    with tarfile.open(path,'r:') as source:
        for member in source:
            value=PurePosixPath(member.name)
            if value.is_absolute() or '..' in value.parts or not(member.isfile() or member.isdir()):raise RuntimeError('Unsafe volume archive')
            total+=member.size
            if total>limit:raise RuntimeError('Volume archive exceeds restore limit')

def decrypt_to_file(argv,archive,target,limit):
    with archive.open('rb') as source,target.open('xb') as output:
        os.chmod(target,0o600)
        with subprocess.Popen(argv,stdin=source,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL) as decrypt:
            timer=threading.Timer(300,decrypt.kill);timer.start()
            try:
                total=0
                while chunk:=decrypt.stdout.read(1<<20):
                    total+=len(chunk)
                    if total>limit:raise RuntimeError('Decrypted archive exceeds restore limit')
                    output.write(chunk)
                if decrypt.wait()!=0:raise RuntimeError('Backup authentication failed; nothing restored')
            except BaseException:decrypt.kill();decrypt.wait();raise
            finally:timer.cancel()

def restore(args,compose,init,up,probe):
    if args.state.exists():raise RuntimeError('Restore requires a new state directory; existing data will not be overwritten')
    if args.archive is None or args.key_file is None:raise RuntimeError('Restore requires --archive and out-of-band --key-file')
    if not args.archive.is_file() or not args.key_file.is_file():raise RuntimeError('Restore archive/key not found')
    # Refuse any existing target container or volume, even if stopped.
    for kind in ['container','volume']:
        check=['docker','ps','-a','-q'] if kind=='container' else ['docker','volume','ls','-q']
        if subprocess.check_output([*check,'--filter','label=com.docker.compose.project='+args.project],text=True).strip():raise RuntimeError('Restore project already owns data: '+args.project)
    image='orchis-server:'+os.environ.get('ORCHIS_VERSION','local')
    args.state.parent.mkdir(parents=True,exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='.orchis-restore-',dir=args.state.parent) as directory:
        staging=Path(directory);plain=staging/'authenticated.zip'
        decrypt_to_file(seal_command(image,args.key_file,True,project=args.project),args.archive,plain,args.max_bytes)
        # Validate the complete authenticated ZIP before creating a destination project.
        with zipfile.ZipFile(plain) as archive:
            members=archive.infolist();names={m.filename for m in members}
            required={'manifest.json','db.dump','volumes/s3.tar','volumes/caddydata.tar','volumes/caddyconfig.tar'}|{'state/'+n for n in STATE_FILES if n not in {'release.key','metrics_token','grafana_password'}}
            if not required<=names or len(members)!=len(names) or len(members)>100000:raise RuntimeError('Incomplete or duplicate backup members')
            if sum(m.file_size for m in members)>args.max_bytes:raise RuntimeError('Expanded backup exceeds restore limit')
            for member in members:
                name=PurePosixPath(member.filename)
                if (member.filename.startswith('state/') and not member.filename.startswith('state/agent-releases/') and member.file_size>65536) or (member.filename=='manifest.json' and member.file_size>1<<20):raise RuntimeError('Oversized backup metadata')
                if name.is_absolute() or '..' in name.parts:raise RuntimeError('Unsafe archive path')
                if member.filename not in required and member.filename not in {'state/release.key','state/metrics_token','state/grafana_password'} and not member.filename.startswith('state/agent-releases/'):raise RuntimeError('Unexpected backup member')
                if (member.external_attr>>16)&0o170000 not in (0,0o100000):raise RuntimeError('Special backup file rejected')
                target=staging/member.filename;target.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
                with archive.open(member) as reader,target.open('xb') as writer:shutil.copyfileobj(reader,writer,1<<20)
                os.chmod(target,0o600)
        manifest=json.loads((staging/'manifest.json').read_text())
        if manifest.get('format')!=1 or manifest.get('project')==args.project:raise RuntimeError('Unsupported backup or non-isolated restore project')
        for name in ['s3','caddydata','caddyconfig']:validate_tar(staging/'volumes'/(name+'.tar'),args.max_bytes)
        init(args) # fresh keys/config/subnet; only selected secret values are restored below
        for name in STATE_FILES:
            source=staging/'state'/name
            if not source.exists():continue
            target=args.state/name
            uid=70 if name in {'postgres_password','owner_password','app_password','maintenance_password'} else 65534 if name=='metrics_token' else 472 if name=='grafana_password' else 999 if name=='rabbitmq.conf' else 1000 if name=='renderer_token' else 0 if name in {'s3_user','s3_password','initial-admin.json','release.key'} else 10001
            target.write_bytes(source.read_bytes());os.chmod(target,0o600);os.chown(target,uid,uid)
        from ops import metrics_init
        metrics_init(args)
        operator=cfg(args);operator['smoke']=bool(manifest.get('smoke'));(args.state/'operator.json').write_text(json.dumps(operator));path=args.state/'server.json';value=json.loads(path.read_text());value['ORCHIS_TRUSTED_PROXIES']=operator['caddy_ip']+'/32';path.write_text(json.dumps(value))
        releases=staging/'state/agent-releases'
        if releases.exists():
            shutil.copytree(releases,args.state/'agent-releases',dirs_exist_ok=True)
            for path in (args.state/'agent-releases').rglob('*'):
                os.chown(path,0 if path.suffix=='.key' else 10001,0 if path.suffix=='.key' else 10001);os.chmod(path,0o700 if path.is_dir() else 0o600 if path.suffix=='.key' else 0o644)
        compose(args,'create','--no-build','pg','minio','caddy')
        pg_image=container(args,compose,'pg')['Config']['Image']
        for name,service,path in [('s3','minio','/data'),('caddydata','caddy','/data'),('caddyconfig','caddy','/config')]:
            vid=volume(args,compose,service,path)
            run(['docker','run','--rm','--label','io.orchis.ops-owner='+args.project,'--network','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true','--memory','128m','--pids-limit','32','--cpus','0.5','-v',vid+':/target','-v',str(staging/'volumes')+':/backup:ro','--entrypoint','/bin/tar',pg_image,'-C','/target','-xf','/backup/'+name+'.tar','--no-same-owner'])
        compose(args,'up','-d','--no-build','--wait','--wait-timeout','90','pg')
        pg_id=container(args,compose,'pg')['Id']
        with (staging/'db.dump').open('rb') as dump:
            run(['docker','exec','-i',pg_id,'sh','-c','set -eu; PGPASSWORD=$(cat /run/secrets/owner_password); export PGPASSWORD; exec pg_restore -h 127.0.0.1 -U orchis -d orchis --no-owner --no-acl --exit-on-error --single-transaction'],stdin=dump,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=180)
        actual=fingerprints(args,compose)
        if actual!=manifest['fingerprints']:raise RuntimeError('Restored business counts or frozen report snapshots differ')
        up(args)
        compose(args,'run','--rm','--no-deps','server','reindex')
        objects=json.loads(compose(args,'run','--rm','--no-deps','server','object-digests',capture=True).stdout)
        if objects!=manifest['pdf_sha256']:raise RuntimeError('Restored PDF objects differ')
        probe(args) # real HTTPS login + PDF rendering; stored report export checked by full smoke
        print('Restore verified in isolated project '+args.project+': '+json.dumps(actual['counts']))
