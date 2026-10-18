"""Regenerate reviewed declarative contracts from a MisterShell OpenAPI snapshot.
Usage: python3 internal/apimanaged/generate_contracts.py /path/to/openapi.json
Shell profiles/mappings are intentionally excluded.
"""
import json,sys
from pathlib import Path
spec=json.load(open(sys.argv[1]));schemas=spec['components']['schemas']
entries=[
('ca_certificate','ca-certificates','cert_id'),
('recording_store','session-policy/recording-stores','store_id'),('recording_rule','session-policy/recording-rules','rule_id'),
('file_store','file-policy/stores','store_id'),('file_transfer_rule','file-policy/rules','rule_id'),
('fact_check','fact-policy/checks','check_id'),('fact_policy_rule','fact-policy/rules','rule_id'),
('automation_playbook','automation/playbooks','playbook_id'),
('configure_template','configure/templates','template_id'),('configure_stack','configure/stacks','stack_id'),('configure_policy','configure/policies','policy_id'),('configure_resource_vars','configure/resource-vars',None),
('proxy','proxies','proxy_id'),('sensor','sensors','sensor_id'),('sensor_custom_rule','sensors/custom-rules','rule_id'),('sensor_alert_rule','sensors/alert-rules','rule_id'),
('sensor_source','sensors/sources/custom','source_id'),('report_template','reporting/templates','template_id'),('command','commands','command_id'),('motd','motds','motd_id')]
def resolve(s):
 while '$ref' in s:s=schemas[s['$ref'].split('/')[-1]]
 return s
def shape(s):
 s=resolve(s)
 if 'anyOf' in s:s=resolve(next(x for x in s['anyOf'] if x.get('type')!='null'))
 return s
def body(op):return resolve(op.get('requestBody',{}).get('content',{}).get('application/json',{}).get('schema',{}))
def response(op):
 for k,v in op['responses'].items():
  if k.startswith('2'):
   env=resolve(v.get('content',{}).get('application/json',{}).get('schema',{}))
   data=shape(env.get('properties',{}).get('data',{}))
   if data.get('type')=='array':data=resolve(data['items'])
   return data
 return {}
result=[]
for name,base,key in entries:
 path='/api/v1/'+base;item=path+'/{id}' if key else path
 realitem=path+'/{'+str(key)+'}' if key else path
 if name=='sensor_source':item='/api/v1/sensors/sources/{id}';realitem=item.replace('{id}','{source_id}')
 create=spec['paths'][path].get('post',spec['paths'][path].get('put'))
 itemops=spec['paths'].get(realitem,{})
 update=itemops.get('patch',itemops.get('put',{}))
 if name=='configure_resource_vars':update=create
 c=body(create);u=body(update);r=response(itemops['get']) if 'get' in itemops else response(create)
 if name=='ca_certificate':r=schemas['CACertificateDetails']
 created=response(create)
 fields=[]
 combined={**c.get('properties',{}),**u.get('properties',{}),**r.get('properties',{})}
 if name in ['proxy','sensor']:combined['token']=created['properties']['token']
 for f,v in sorted(combined.items()):
  if f=='id':continue
  cv=c.get('properties',{}).get(f);uv=u.get('properties',{}).get(f)
  write=cv is not None or uv is not None
  original=cv if cv is not None else v
  typ=shape(original).get('type','object');typ=typ if typ in ['string','integer','number','boolean'] else 'dynamic'
  a={'name':f,'kind':typ,'create':cv is not None,'update':uv is not None,'required':f in c.get('required',[]),'sensitive':f in ['token','parameters','values'],'description':original.get('description',f.replace('_',' ').capitalize()+'.')}
  if name=='automation_playbook' and f=='graph':
   a.update(sensitive=True,secret_keys=['token','password','header_value'])
  if write and not a['required']:
   if cv is not None and 'default' in cv:a['default']=cv['default']
   elif cv is not None and shape(cv).get('type')=='array':a['default']=[]
   elif cv is not None and shape(cv).get('type')=='object' and 'anyOf' not in cv:a['default']={}
   elif cv is None:a['computed']=True
  if cv is not None and uv is None:a['immutable']=True
  if f=='position':a.update(computed=True)
  if name=='recording_rule' and f in ['retention_days','destination_store_id']:
   a.update(computed=True);a.pop('default',None)
  scalar=shape(original)
  for src,dst in [('enum','enum'),('minLength','min_length'),('maxLength','max_length')]:
   if src in scalar:a[dst]=scalar[src]
  fields.append(a)
 if name=='ca_certificate':
  for f in fields:
   if f['name']=='enabled':f.update(default=True,computed=False)
 if name=='configure_resource_vars':
  for f in fields:
   if f['name'] in ['resource_id','stack_id']:f['immutable']=True
 if name=='sensor_source':
  for f in fields:
   if f['name']=='enabled':f.update(update=True,default=False)
   if f['name']=='parameters':f.update(update=True,computed=True,sensitive=True)
 result.append({'name':name,'list_only':'get' not in itemops,'path':path,'item_path':item,'method':'PUT' if name=='configure_resource_vars' else 'POST','update_method':'PUT' if 'put' in itemops or name=='configure_resource_vars' else 'PATCH','fields':fields,'description':'Manages a MisterShell '+name.replace('_',' ')+'.','list_path':'/api/v1/sensors/sources' if name=='sensor_source' else path})
Path(__file__).with_name('contracts.json').write_text(json.dumps(result,indent=2)+'\n')
print('Generated',len(result),'reviewable API contracts')
