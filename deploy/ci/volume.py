#!/usr/bin/env python3
"""Bounded synthetic volume study; only unique resources recorded by this run are removed."""
import json,os,re,secrets,shutil,subprocess,time,urllib.request,urllib.error
from pathlib import Path
root=Path(__file__).resolve().parents[2]
if shutil.disk_usage(root).free<4<<30:raise SystemExit('Volume study requires at least 4 GiB free disk')
folder=root/'work'/'volume'/(time.strftime('%Y%m%dT%H%M%S')+'-'+secrets.token_hex(4));folder.mkdir(mode=0o700,parents=True)
marker=folder/'owned.json';output=folder/'result.json'
env={**os.environ,'ORCHIS_VOLUME_TEST':'1','ORCHIS_VOLUME_MARKER':str(marker),'ORCHIS_VOLUME_OUTPUT':str(output),'ORCHIS_TEST_PG_URL':'postgres://orchis:orchis@127.0.0.1:15432/postgres'}
status=1
try:
 status=subprocess.run(['go','test','-v','-count=1','-timeout=15m','./e2e/volume'],cwd=root,env=env,timeout=960).returncode
finally:
 if marker.exists():
  owned=json.loads(marker.read_text());database=owned.get('database','');index=owned.get('es_index','')
  if not re.fullmatch(r'orchis_test_[a-f0-9]{12}',database):raise RuntimeError('Refuse unexpected database cleanup name')
  subprocess.run(['docker','compose','-f',str(root/'deploy/compose.yml'),'exec','-T','pg','psql','-U','orchis','-d','postgres','-v','ON_ERROR_STOP=1','-c','DROP DATABASE IF EXISTS '+database+' WITH (FORCE)'],check=True,timeout=30,stdout=subprocess.DEVNULL)
  if index:
   if not re.fullmatch(r'orchis-volume-[a-f0-9]{32}',index):raise RuntimeError('Refuse unexpected search index cleanup name')
   try:
    with urllib.request.urlopen(urllib.request.Request('http://127.0.0.1:19200/'+index,method='DELETE'),timeout=15) as response:response.read(4096)
   except urllib.error.HTTPError as error:
    if error.code!=404:raise
  marker.unlink();(folder/'cleanup.json').write_text(json.dumps({'database_removed':True,'search_index_removed':bool(index)}))
 print('Volume evidence: '+str(folder))
raise SystemExit(status)
