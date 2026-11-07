#!/usr/bin/env python3
"""Deterministic transport fixture for the explicit production smoke profile only.
No target, provider, mailbox or model requests are made. Never used in production.
"""
import argparse,json,os,sys,uuid
from pathlib import Path
parser=argparse.ArgumentParser();parser.add_argument('--inputs-dir');parser.add_argument('--dir',required=True);parser.add_argument('--prompt-file',required=True);args=parser.parse_args()
if args.prompt_file!='-':raise SystemExit('smoke fixture requires stdin')
sys.stdin.read()
if not args.inputs_dir:raise SystemExit('missing preliminary scans')
for tool in ['nmap','nuclei']:
 data=json.loads((Path(args.inputs_dir)/(tool+'.json')).read_text());assert data['tool']==tool

session=str(uuid.uuid4());root=Path(args.dir);root.mkdir(parents=True,exist_ok=True)
result={'schema':1,'session_id':session,'disposition':'limited','phase':'report','summary':'Synthetic integration smoke only: verified queue dispatch, job result transport, report persistence and PDF generation. The preliminary scanner observations were handed off. No Hackbot model testing was performed.','phases':[],'verified_tasks':0,'blockers':['Synthetic protocol fixture; no pentest coverage.'],'findings':[],'report':'Synthetic integration fixture; no pentest performed.','validation':'No vulnerability candidates; synthetic integration fixture.'}
file=root/'service-result.json';file.write_text(json.dumps(result));file.chmod(0o600)
for event in [{'event':'started'},{'event':'state','phase':'report'},{'event':'limited','resultFile':str(file)}]:
 print(json.dumps({'schema':1,'sessionId':session,**event}),flush=True)
sys.exit(2)
