// Package mcpapps provides embedded MCP Apps UI resources for server tools.
package mcpapps

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

const namespacesListURI = "ui://kubernetes-mcp-server/namespaces-list"

// NamespacesList returns the self-contained application associated with
// namespaces_list. It intentionally has no network dependencies so it also
// works in disconnected clusters and under the default restrictive CSP.
func NamespacesList() *api.ToolApp {
	return &api.ToolApp{
		URI:         namespacesListURI,
		Name:        "Namespaces list",
		Description: "Interactive table of Kubernetes namespaces",
		Meta: map[string]any{
			"ui": map[string]any{"prefersBorder": true},
		},
		Handler: func(context.Context) (string, error) { return namespacesListHTML, nil },
	}
}

const namespacesListHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Namespaces</title><style>
body{font:14px system-ui,sans-serif;margin:0;padding:12px;color:#1f2937;background:#fff}table{width:100%;border-collapse:collapse}th,td{padding:8px;text-align:left;border-bottom:1px solid #d1d5db}th{cursor:pointer;background:#f9fafb}tr:nth-child(even){background:#f9fafb}.status{color:#6b7280;font-style:italic}
</style></head><body><div id="app" class="status">Loading namespaces…</div><script>
window.addEventListener('message',e=>{if(e.source!==parent)e.stopImmediatePropagation()},true);
(()=>{const app=document.getElementById('app');let rows=[],sortColumn='Name';const preferredColumns=['Name','Status','Age','Labels','apiVersion','kind'];function fail(message){app.className='status';app.textContent='Unable to load namespaces: '+(message||'unknown error')}function rowsFrom(data){const result=Array.isArray(data)?data:data&&data.items;if(!Array.isArray(result))throw new Error('expected a list of namespace rows');return result.map((row,index)=>{if(!row||typeof row!=='object'||Array.isArray(row))throw new Error('namespace row '+(index+1)+' is invalid');return row})}function render(data){rows=rowsFrom(data);rows.sort((a,b)=>String(a[sortColumn]??'').localeCompare(String(b[sortColumn]??'')));if(!rows.length){app.className='status';app.textContent='No namespaces found.';return}const keys=new Set(rows.flatMap(Object.keys));const cols=[...preferredColumns.filter(c=>keys.delete(c)),...[...keys].sort()];app.className='';app.innerHTML='<table><thead><tr>'+cols.map(c=>'<th>'+escape(c)+'</th>').join('')+'</tr></thead><tbody>'+rows.map(r=>'<tr>'+cols.map(c=>'<td>'+escape(String(r[c]??''))+'</td>').join('')+'</tr>').join('')+'</tbody></table>';app.querySelectorAll('th').forEach((h,i)=>h.onclick=()=>{sortColumn=cols[i];render(rows)})}function escape(s){const e=document.createElement('span');e.textContent=s;return e.innerHTML}function send(m){parent.postMessage(m,'*')}function toolError(params){const content=params&&params.content;if(Array.isArray(content)){const text=content.filter(item=>item&&item.type==='text').map(item=>item.text).join(' ');if(text)return text}return 'the namespace request failed'}window.addEventListener('message',e=>{try{const m=e.data;if(!m||m.jsonrpc!=='2.0')return;if(m.id===1){if(m.error){fail(m.error.message||'initialization failed');return}if(m.result)send({jsonrpc:'2.0',method:'ui/notifications/initialized',params:{}});return}if(m.method==='ui/notifications/tool-result'){const params=m.params||{};if(params.isError){fail(toolError(params));return}if(!Object.prototype.hasOwnProperty.call(params,'structuredContent')){fail('the server returned no structured namespace data');return}render(params.structuredContent);return}if(m.id&&m.method==='ui/resource-teardown')send({jsonrpc:'2.0',id:m.id,result:{}})}catch(err){fail(err&&err.message||'invalid server response')}});send({jsonrpc:'2.0',id:1,method:'ui/initialize',params:{protocolVersion:'2026-01-26',capabilities:{},clientInfo:{name:'kubernetes-mcp-server',version:'1.0.0'}}})})();
</script></body></html>`
