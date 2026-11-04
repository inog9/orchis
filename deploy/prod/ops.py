#!/usr/bin/env python3
"""Operate one explicitly selected production project; never discover/modify other stacks."""
import argparse, contextlib, hashlib, fcntl, base64, http.cookiejar, ipaddress, json, os, re, secrets, ssl, subprocess, time, urllib.request
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]

def private(path, value, uid=10001):
    data = (json.dumps(value, indent=2)+"\n") if isinstance(value,dict) else value+"\n"
    fd = os.open(path, os.O_WRONLY|os.O_CREAT|os.O_EXCL, 0o600)
    with os.fdopen(fd,"w") as out: out.write(data)
    os.chown(path,uid,uid)

def init(args):
    if os.geteuid()!=0: raise SystemExit("prod-init needs root to assign private secret files to container users")
    if args.state.exists(): raise SystemExit("State directory exists; refusing to replace secrets")
    if not re.fullmatch(r"[a-z][a-z0-9-]{2,39}",args.project): raise SystemExit("Invalid project name")
    from mitre_config import defaults
    mitre=defaults(REPO)
    network_ids=subprocess.check_output(["docker","network","ls","-q"],text=True).split()
    networks=json.loads(subprocess.check_output(["docker","network","inspect",*network_ids],text=True)) if network_ids else []
    used=[ipaddress.ip_network(c["Subnet"]) for n in networks for c in (n.get("IPAM",{}).get("Config") or []) if c.get("Subnet")]
    subnet=next((ipaddress.ip_network(f"172.29.{i}.0/24") for i in range(256) if not any(ipaddress.ip_network(f"172.29.{i}.0/24").overlaps(n) for n in used if n.version==4)),None)
    if subnet is None:raise SystemExit("No free proxy subnet; configure an explicit network")
    caddy_ip=str(subnet.network_address+10)
    args.state.mkdir(mode=0o700,parents=True)
    random=lambda:secrets.token_urlsafe(32)
    owner,app,maint,postgres,mq,s3,admin,renderer,ai,nmap,nuclei,metrics=[random() for _ in range(12)]
    nmap="oij_"+nmap; nuclei="oij_"+nuclei
    for name,value,uid in [("postgres_password",postgres,70),("owner_password",owner,70),("app_password",app,70),("maintenance_password",maint,70),("s3_user","orchis",0),("s3_password",s3,0),("renderer_token",renderer,1000),("ai-master.key",base64.b64encode(secrets.token_bytes(32)).decode(),10001)]:
        private(args.state/name,value,uid)
    private(args.state/"metrics_token",metrics,65534)
    (args.state/"ops-status").mkdir(mode=0o755)
    private(args.state/"grafana_password",random(),472)
    private(args.state/"rabbitmq.conf",f"default_user = orchis\ndefault_pass = {mq}\nloopback_users.guest = true\nvm_memory_high_watermark.absolute = 192MiB\nmanagement.tcp.ip = 127.0.0.1",999)
    base={"ORCHIS_PRODUCTION":"true","ORCHIS_TENANT":"default","ORCHIS_COOKIE_SECURE":"true","ORCHIS_TRUSTED_PROXIES":caddy_ip+"/32",**mitre}
    url=lambda role,pw:f"postgres://{role}:{pw}@pg:5432/orchis?sslmode=disable"
    appbase={**base,"ORCHIS_DATABASE_URL":url("orchis_app",app)}
    private(args.state/"migrate.json",{**base,"ORCHIS_MIGRATE_URL":url("orchis",owner)})
    private(args.state/"provision.json",{**appbase,"ORCHIS_PROVISION_NMAP_TOKEN":nmap,"ORCHIS_PROVISION_NUCLEI_TOKEN":nuclei})
    mqurl=f"amqp://orchis:{mq}@mq:5672/"
    private(args.state/"server.json",{**appbase,"ORCHIS_METRICS_TOKEN":metrics,"ORCHIS_OPS_STATUS_FILE":"/run/ops-status/backup.json","ORCHIS_HTTP_ADDR":"0.0.0.0:8090","ORCHIS_WEB_DIR":"/web","ORCHIS_ADMIN_EMAIL":args.email,"ORCHIS_ADMIN_PASSWORD":admin,"ORCHIS_AMQP_URL":mqurl,"ORCHIS_ES_URL":"http://es:9200","ORCHIS_S3_ENDPOINT":"minio:9000","ORCHIS_S3_ACCESS_KEY":"orchis","ORCHIS_S3_SECRET_KEY":s3,"ORCHIS_AI_WORKER_TOKEN":ai,"ORCHIS_AI_ENCRYPTION_KEY_FILE":"/run/secrets/ai-master.key","ORCHIS_RENDERER_URL":"http://renderer:8091/render","ORCHIS_RENDERER_TOKEN":renderer,"ORCHIS_AGENT_RELEASE_DIR":"/releases"})
    for kind,token in [("nmap",nmap),("nuclei",nuclei)]:private(args.state/(kind+".json"),{"ORCHIS_METRICS_TOKEN":metrics,"ORCHIS_SERVER_URL":"http://server:8090","ORCHIS_INJECTOR_TOKEN":token,"ORCHIS_AMQP_URL":mqurl})
    private(args.state/"ai.json",{"ORCHIS_METRICS_TOKEN":metrics,"ORCHIS_SERVER_URL":"http://server:8090","ORCHIS_AI_WORKER_TOKEN":ai,"ORCHIS_AI_PROXY":"http://ai-proxy:8080"})
    private(args.state/"initial-admin.json",{"email":args.email,"password":admin},0)
    private(args.state/"operator.json",{"project":args.project,"port":str(args.port),"bind":"127.0.0.1","site":"localhost","proxy_subnet":str(subnet),"proxy_range":str(subnet.network_address+128)+"/25","caddy_ip":caddy_ip,"smoke":args.command=="smoke"},0)
    private(args.state/"backup.key",secrets.token_hex(32),0)
    (args.state/"agent-releases").mkdir(mode=0o755)
    print("Initialized private state. Initial credentials: "+str(args.state/"initial-admin.json"))

def metrics_init(args):
    """Explicit upgrade of private metric configuration; preserves other credentials."""
    if os.geteuid()!=0:raise RuntimeError('Metrics setup requires the root operator')
    token_path=args.state/'metrics_token'
    if not token_path.exists():private(token_path,secrets.token_urlsafe(32),65534)
    if token_path.is_symlink() or not token_path.is_file() or token_path.stat().st_mode&0o077 or token_path.stat().st_size>128:raise RuntimeError('Metrics token must be a private regular file')
    token=token_path.read_text().strip()
    if len(token)<32:raise RuntimeError('Metrics token is too short')
    os.chown(token_path,65534,65534)
    for name in ['server.json','nmap.json','nuclei.json','ai.json']:
        path=args.state/name
        if path.is_symlink() or not path.is_file() or path.stat().st_mode&0o077 or path.stat().st_size>65536:raise RuntimeError('Application configuration must be a private regular file')
        value=json.loads(path.read_text());value['ORCHIS_METRICS_TOKEN']=token
        if name=='server.json':value['ORCHIS_OPS_STATUS_FILE']='/run/ops-status/backup.json'
        temporary=args.state/(name+'.metrics-'+secrets.token_hex(4))
        try:
            private(temporary,value)
            os.replace(temporary,path)
        finally:temporary.unlink(missing_ok=True)
    password=args.state/'grafana_password'
    if not password.exists():private(password,secrets.token_urlsafe(32),472)
    if password.is_symlink() or not password.is_file() or password.stat().st_mode&0o077:raise RuntimeError('Grafana password must be a private regular file')
    os.chown(password,472,472)
    status=args.state/'ops-status'
    if status.is_symlink():raise RuntimeError('Metrics status directory cannot be a symlink')
    status.mkdir(mode=0o755,exist_ok=True);os.chmod(status,0o755)

def compose(args,*tail,capture=False,input=None):
    cfg=json.loads((args.state/"operator.json").read_text())
    env={**os.environ,"ORCHIS_STATE":str(args.state),"ORCHIS_PROJECT":cfg["project"],"ORCHIS_HTTPS_PORT":cfg["port"],"ORCHIS_BIND":cfg["bind"],"ORCHIS_SITE":cfg["site"],"ORCHIS_PROXY_SUBNET":cfg["proxy_subnet"],"ORCHIS_PROXY_RANGE":cfg["proxy_range"],"ORCHIS_CADDY_IP":cfg["caddy_ip"]}
    files=["-f",str(REPO/"docker-compose.yaml"),"-f",str(REPO/"deploy/prod/compose.ops.yml")]
    if cfg.get("smoke"):files += ["-f",str(REPO/"deploy/prod/compose.smoke.yml")]
    return subprocess.run(["docker","compose","--project-directory",str(REPO),*files,*tail],env=env,check=True,text=True,capture_output=capture,input=input,timeout=1200 if "build" in tail else 300)

def env_file(args):
    """Print the non-secret values `docker compose` needs, for `.env`; secrets stay in the private state directory."""
    cfg=json.loads((args.state/"operator.json").read_text())
    print("\n".join(f"{k}={v}" for k,v in [("ORCHIS_STATE",args.state),("ORCHIS_PROJECT",cfg["project"]),("ORCHIS_BIND",cfg["bind"]),("ORCHIS_HTTPS_PORT",cfg["port"]),("ORCHIS_SITE",cfg["site"]),("ORCHIS_PROXY_SUBNET",cfg["proxy_subnet"]),("ORCHIS_PROXY_RANGE",cfg["proxy_range"]),("ORCHIS_CADDY_IP",cfg["caddy_ip"])]))

def ca_fingerprint(args):
    """SHA-256 of the self-signed installation's root CA, for pinning in install.sh (--ca-fingerprint)."""
    pem=compose(args,"exec","-T","caddy","cat","/data/caddy/pki/authorities/local/root.crt",capture=True).stdout
    print(hashlib.sha256(ssl.PEM_cert_to_DER_cert(pem)).hexdigest())

def agent_trust(args):
    """Publish the installation's own CA fingerprint to authenticated installer clients."""
    pem=compose(args,"exec","-T","caddy","cat","/data/caddy/pki/authorities/local/root.crt",capture=True).stdout
    fingerprint=hashlib.sha256(ssl.PEM_cert_to_DER_cert(pem)).hexdigest()
    path=args.state/"server.json"
    value=json.loads(path.read_text())
    value["ORCHIS_AGENT_CA_FINGERPRINT"]=fingerprint
    value.pop("ORCHIS_AGENT_TLS_PIN",None)
    temporary=args.state/("server.json.trust-"+secrets.token_hex(4))
    try:
        private(temporary,value)
        os.replace(temporary,path)
    finally:temporary.unlink(missing_ok=True)
    print("Internal CA fingerprint configured. Restart the server explicitly to apply.")

def up(args):
    if (args.state/"ai-rekey.json").exists():raise RuntimeError("AI key rotation pending; resume ai-rekey before startup")
    compose(args,"up","-d","--no-build")
    deadline=time.monotonic()+180
    while time.monotonic()<deadline:
        ids=compose(args,"ps","-a","-q",capture=True).stdout.split()
        states=json.loads(subprocess.check_output(["docker","inspect",*ids],text=True)) if ids else []
        pending=[]
        for item in states:
            service=item["Config"]["Labels"]["com.docker.compose.service"]
            state=item["State"]
            if service in ["migrate","provision"]:
                if state["Status"]=="exited" and state["ExitCode"]==0:continue
                if state["Status"]=="exited":raise SystemExit("Setup job failed: "+service)
            elif state.get("Running"):
                health=state.get("Health")
                if not health or health.get("Status")=="healthy":continue
                if health.get("Status")=="unhealthy":raise SystemExit("Service unhealthy: "+service)
            pending.append(service)
        if states and not pending:
            tenant=json.loads((args.state/"server.json").read_text())["ORCHIS_TENANT"].replace("'","''")
            sql="SELECT count(*) FROM integrations i JOIN tenants t ON t.id=i.tenant_id WHERE t.name='"+tenant+"' AND i.key IN ('nmap','nuclei') AND (NOT i.enabled OR i.last_heartbeat_at>now()-interval '45 seconds')"
            count=compose(args,"exec","-T","pg","psql","-U","postgres","-d","orchis","-At","-v","ON_ERROR_STOP=1","-c",sql,capture=True).stdout.strip()
            if count=="2":
                print("Project readiness: OK (core health and authenticated injector heartbeats; explicitly disabled integrations excluded)")
                return
            pending=["authenticated injector heartbeats"]
        time.sleep(2)
    raise SystemExit("Readiness timeout: "+", ".join(pending))

def probe(args):
    cfg=json.loads((args.state/"operator.json").read_text())
    if cfg["site"]!="localhost" or cfg["bind"]!="127.0.0.1":raise SystemExit("This probe is restricted to local HTTPS")
    cert=compose(args,"exec","-T","caddy","cat","/data/caddy/pki/authorities/local/root.crt",capture=True).stdout
    context=ssl.create_default_context(cadata=cert)
    jar=http.cookiejar.CookieJar()
    client=urllib.request.build_opener(urllib.request.HTTPSHandler(context=context),urllib.request.HTTPCookieProcessor(jar))
    base="https://localhost:"+cfg["port"]
    with client.open(base+"/api/healthz",timeout=10) as res:
        assert res.status==200 and res.headers.get("Strict-Transport-Security"),"HTTPS health/security headers failed"
    credential=json.loads((getattr(args,"credentials_file",None) or args.state/"initial-admin.json").read_text())
    request=urllib.request.Request(base+"/api/v1/auth/login",data=json.dumps(credential).encode(),headers={"Content-Type":"application/json","X-Requested-With":"orchis-web","X-Forwarded-For":"198.51.100.77"})
    with client.open(request,timeout=10) as res:
        cookie=res.headers.get("Set-Cookie","")
        assert res.status==200 and all(flag in cookie for flag in ["Secure","HttpOnly","SameSite=Lax"]),"Session cookie flags failed"
    with client.open(base+"/api/v1/auth/me",timeout=10) as res:
        assert json.load(res)["email"]==credential["email"],"HTTPS session failed"
    if json.loads((args.state/"server.json").read_text()).get("ORCHIS_MITRE_URL") != "off":
        from mitre_config import verify_import
        verify_import(client,base,REPO)
    else:
        print("ATT&CK import explicitly disabled in this installation")
    result=compose(args,"exec","-T","renderer","node","--input-type=module",capture=True,input=(REPO/"deploy/prod/renderer-probe.mjs").read_text())
    print("HTTPS health, trusted CA, secure cookie and login: OK")
    print(result.stdout.strip())
    print("Infrastructure probe only; run prod-smoke for the full assessment and encrypted restore test")

@contextlib.contextmanager
def state_lock(state):
    fd=os.open(state/".ops.lock",os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
    try:
        try:fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
        except BlockingIOError:raise SystemExit("Another operation is using this state directory")
        yield
    finally:os.close(fd)

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument("command",choices=["init","build","up","down","status","probe","backup-key","backup","restore","smoke","metrics-init","agent-trust","mitre-config","ai-rekey","demo-seed","demo-purge","demo-status","env","ca-fingerprint"])
    p.add_argument("--state",type=Path,default=None)
    p.add_argument("--project")
    p.add_argument("--port",type=int)
    p.add_argument("--email",default="admin@orchis.local")
    p.add_argument("--demo-seed",type=int,default=int(os.environ.get("DEMO_SEED","1")))
    p.add_argument("--credentials-file",type=Path)
    p.add_argument("--archive",type=Path,default=Path(os.environ["ORCHIS_BACKUP_FILE"]) if os.environ.get("ORCHIS_BACKUP_FILE") else None)
    p.add_argument("--key-file",type=Path,default=Path(os.environ["ORCHIS_BACKUP_KEY_FILE"]) if os.environ.get("ORCHIS_BACKUP_KEY_FILE") else None)
    p.add_argument("--retention-days",type=int,default=int(os.environ.get("ORCHIS_BACKUP_RETENTION_DAYS","7")))
    p.add_argument("--max-bytes",type=int,default=8<<30)
    args=p.parse_args()
    args.state=args.state or Path(os.environ.get("ORCHIS_STATE","/var/lib/orchis-smoke" if args.command=="smoke" else "/var/lib/orchis-restore" if args.command=="restore" else "/var/lib/orchis"))
    args.project=args.project or ("orchis-smoke" if args.command=="smoke" else "orchis-restore" if args.command=="restore" else "orchis-prod")
    args.port=args.port if args.port is not None else (8445 if args.command=="smoke" else 8444 if args.command=="restore" else 8443)
    if not re.fullmatch(r"[a-z][a-z0-9-]{2,39}",args.project):p.error("Invalid project name")
    if not 1<=args.retention_days<=3650 or not 1<<20<=args.max_bytes<=1<<40:p.error("Invalid retention or archive size limit")
    if not args.state.is_absolute():p.error("State must be absolute")
    args.state=args.state.resolve()
    if args.key_file and args.key_file.resolve().is_relative_to(REPO):p.error("Backup keys must be outside the checkout")
    if not args.state.is_absolute() or args.state.is_relative_to(REPO):p.error("State must be an absolute path outside the checkout")
    if not 1024<=args.port<=65535:p.error("Port must be between 1024 and 65535")
    def execute():
        if args.command=="init":init(args)
        elif args.command in {"demo-seed","demo-purge","demo-status"}:
            if not 0<=args.demo_seed<=1000000:raise RuntimeError("DEMO_SEED must be between 0 and 1000000")
            compose(args,"run","--rm","--no-deps","-e","DEMO_SEED="+str(args.demo_seed),"server",args.command)
        elif args.command=="probe":probe(args)
        elif args.command=="env":env_file(args)
        elif args.command=="ca-fingerprint":ca_fingerprint(args)
        elif args.command=="metrics-init":metrics_init(args)
        elif args.command=="agent-trust":agent_trust(args)
        elif args.command=="mitre-config":
            from mitre_config import configure
            configure(args,REPO,private)
        elif args.command=="build":
            compose(args,"build","server","renderer")
            compose(args,"build","nmap")
        elif args.command=="up":up(args)
        elif args.command=="down":compose(args,"down","--timeout","30")
        elif args.command=="backup-key":
            path=args.state/"backup.key"
            if path.exists():print("Existing backup key retained")
            else:private(path,secrets.token_hex(32),0);print("Created separate backup key; copy it to protected out-of-band storage")
        elif args.command=="ai-rekey":
            from rekey import rekey
            rekey(args,compose)
        elif args.command=="backup":
            from backup import backup
            backup(args,compose)
        elif args.command=="restore":
            from backup import restore
            restore(args,compose,init,up,probe)
        elif args.command=="smoke":
            from smoke import smoke
            smoke(args,compose,init,up,probe)
        else:compose(args,"ps","-a")
    if args.command in {"up","down","build","backup-key","backup","metrics-init","agent-trust","mitre-config","ai-rekey","demo-seed","demo-purge","demo-status"}:
        with state_lock(args.state):execute()
    else:execute()

if __name__=="__main__":main()
