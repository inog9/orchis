#!/usr/bin/env python3
"""Periksa teks, cakupan, penanda demo, nomor halaman dan batas A4; tetap perlu tinjauan PNG setiap halaman."""
import argparse,json,re,subprocess,xml.etree.ElementTree as ET
from pathlib import Path

def normalize(text):return re.sub(r'\s+',' ',text).strip()
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('pdf',type=Path);p.add_argument('snapshot',type=Path);args=p.parse_args()
 s=json.loads(args.snapshot.read_text());s=s.get('snapshot',s)
 xml=subprocess.check_output(['pdftotext','-bbox',str(args.pdf),'-'],timeout=30)
 root=ET.fromstring(xml);pages=root.findall('.//{*}page');assert pages,'PDF has no pages'
 texts=[]
 for number,page in enumerate(pages,1):
  width,height=float(page.attrib['width']),float(page.attrib['height'])
  assert abs(width-595)<3 and abs(height-842)<3,'PDF is not A4'
  words=page.findall('.//{*}word');assert len(words)>10,'Blank or almost empty page'
  text=normalize(' '.join(w.text or '' for w in words));texts.append(text)
  assert f'{number} / {len(pages)}' in text,'Missing or incorrect page number'
  caption='DEMO / SYNTHETIC report - not a real scan' if s.get('is_demo') else 'Frozen organisation report'
  assert caption in text,'Missing per-page provenance caption'
  for word in words:
   x1,y1,x2,y2=[float(word.attrib[k]) for k in ['xMin','yMin','xMax','yMax']]
   assert x1>=20 and y1>=10 and x2<=width-20 and y2<=height-5,'Text extends past safe page bounds'
 text=normalize(' '.join(texts));assets=s['assets']
 skipped=sum(a.get('status')=='skipped' or bool(a.get('skip_reason')) for a in assets)
 tested=sum(a.get('status')=='finished' and bool(a.get('methods')) and all(m['outcome']=='success' for m in a['methods']) and not a.get('skip_reason') for a in assets)
 assert f'{len(assets)} assets in scope · {tested} fully tested · {skipped} skipped · {len(assets)-tested-skipped} incomplete' in text,'Summary coverage differs from frozen snapshot'
 for asset in assets:
  assert normalize(asset['name']) in text,'Asset missing from PDF'
  for finding in asset.get('findings',[]):assert normalize(finding['title']) in text,'Finding missing from PDF'
 assert normalize(s['organization']) in text,'Organisation missing'
 print(json.dumps({'pages':len(pages),'assets':len(assets),'tested':tested,'skipped':skipped,'incomplete':len(assets)-tested-skipped,'findings':sum(len(a.get('findings',[])) for a in assets),'synthetic':bool(s.get('is_demo')),'safe_bounds':True,'manual_png_review_required':True},indent=2))
if __name__=='__main__':main()
