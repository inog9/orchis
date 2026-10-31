#!/usr/bin/env python3
"""Buktikan SIGTERM membersihkan server, DB dan injector fixture milik satu run."""
from pathlib import Path
import os,signal,socket,subprocess,time
root=Path(__file__).resolve().parents[1]
def docker(*args):return subprocess.check_output(['docker',*args],cwd=root,text=True)
def containers():return set(n for n in docker('ps','--format','{{.Names}}').splitlines() if n.startswith(('orchis-e2e-nmap-','orchis-e2e-nuclei-')))
def databases():
 return set(docker('compose','-f','deploy/compose.yml','exec','-T','pg','psql','-U','orchis','-d','postgres','-At','-c',"SELECT datname FROM pg_database WHERE datname LIKE 'orchis_e2e_%'").splitlines())
def freeport():
 with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
port=freeport();target_port=freeport()
while target_port==port:target_port=freeport()
def server_alive():
 with socket.socket() as s:s.settimeout(.2);return s.connect_ex(('127.0.0.1',port))==0
before_containers=containers();before_db=databases();log=root/'work'/'cleanup-signal.log'
with log.open('w') as out:
 p=subprocess.Popen(['bash','e2e/run.sh'],cwd=root,env={**os.environ,'E2E_SCRIPT':'flow3.mjs','E2E_PORT':str(port),'E2E_TARGET_PORT':str(target_port)},stdout=out,stderr=subprocess.STDOUT,start_new_session=True)
 try:
  deadline=time.monotonic()+45
  while time.monotonic()<deadline:
   if p.poll() is not None:raise RuntimeError('Fixture stopped before signal test; see '+str(log))
   if len(containers()-before_containers)==2:break
   time.sleep(.2)
  else:raise RuntimeError('Injector startup timed out')
  os.killpg(p.pid,signal.SIGTERM)
  p.wait(timeout=25)
  deadline=time.monotonic()+15
  while time.monotonic()<deadline:
   if not (containers()-before_containers) and not (databases()-before_db) and not server_alive():break
   time.sleep(.3)
  else:raise RuntimeError('Signal leaked fixture containers or database; see '+str(log))
  print('LULUS: SIGTERM membersihkan server, dua injector dan database fixture, tanpa menyentuh resource lain.')
 finally:
  if p.poll() is None:
   os.killpg(p.pid,signal.SIGTERM)
   p.wait(timeout=25)
