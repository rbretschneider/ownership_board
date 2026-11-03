import {useEffect, useId, useRef, useState} from "react";
import {Minus, Plus} from "lucide-react";
import {useApi} from "../../api/ApiContext";
import type {BoardSettings} from "../../api/types";
import {ErrorState} from "../common/ErrorState";
import {useToast} from "../toast/ToastContext";

type AccountField = "codex_workers" | "claude_workers" | "codex_weekly_limit" | "claude_weekly_limit";
export function AgentWorkSection() {
 const api=useApi(), toast=useToast(), id=useId();
 const [settings,setSettings]=useState<BoardSettings|null>(null);
 const [error,setError]=useState<string|null>(null);
 const [busy,setBusy]=useState(false);
 const confirmed=useRef<BoardSettings|null>(null);
 const desired=useRef<Partial<BoardSettings>>({});
 const latest=useRef<BoardSettings|null>(null);
 const saving=useRef(false);
 const normalize=(s:BoardSettings):BoardSettings=>({...s,claude_workers:s.claude_workers??0,claude_worker_limit:s.claude_worker_limit??6,codex_weekly_limit:s.codex_weekly_limit??100,claude_weekly_limit:s.claude_weekly_limit??100});
 useEffect(()=>{let active=true; void api.settings().then(s=>{if(active){confirmed.current=normalize(s);latest.current=confirmed.current;setSettings(confirmed.current);}},e=>{if(active)setError(String(e));});return()=>{active=false;};},[api]);
 const change=async (patch:Partial<BoardSettings>)=>{
  if(!confirmed.current)return;
  desired.current={...desired.current,...patch};
  latest.current={...latest.current!,...patch};
  setSettings(latest.current);
  if(saving.current)return;
  saving.current=true;setBusy(true);
  try {
   while(Object.keys(desired.current).length){
    const target={...desired.current};desired.current={};
    confirmed.current=normalize(await api.editSettings(target));
    latest.current={...confirmed.current,...desired.current};
    setSettings(latest.current);
   }
  } catch(e) {
   desired.current={};
   try{confirmed.current=normalize(await api.settings());latest.current=confirmed.current;setSettings(confirmed.current);}catch{setSettings(null);setError("Settings are unconfirmed. Reload to see the current values.");}
   toast.error(`Couldn't save settings: ${String(e)}`);
  } finally {saving.current=false;setBusy(false);}
 };
 const adjust=(provider:"codex"|"claude",value:number)=>{
  if(!settings||!Number.isInteger(value))return;
  const field=`${provider}_workers` as AccountField;
  const maximum=settings[`${provider}_worker_limit`];
  const count=Math.max(0,Math.min(maximum,value));
  // Expand capacity when needed; reducing one account never constrains the other.
  const other=provider==="codex"?(latest.current?.claude_workers??settings.claude_workers):(latest.current?.codex_workers??settings.codex_workers);
  const patch:Partial<BoardSettings>={[field]:count};
  if(count+other>(latest.current?.autonomous_in_flight_cap??settings.autonomous_in_flight_cap))patch.autonomous_in_flight_cap=Math.min(10,count+other);
  void change(patch);
 };
 return <section className="settings-section" aria-labelledby={`${id}-title`}>
  <h3 id={`${id}-title`} className="settings-section__title">Agent work</h3>
  {error&&<ErrorState message={error} onRetry={()=>window.location.reload()}/>}
  {settings&&<div className="field">
   <label className="switch"><input type="checkbox" role="switch" className="switch__input" checked={settings.windows_runner_enabled} disabled={busy} onChange={e=>void change({windows_runner_enabled:e.target.checked})}/><span className="switch__track" aria-hidden="true"/><span className="switch__label">Enable Windows runner</span></label>
   <p className="muted small">Allow Windows checks for every repository. Turning this off stops Windows builds and holds queued work. Runs hidden at low priority, with at most two build/test jobs.</p>
   {(["codex","claude"] as const).map(provider=>{
    const label=provider==="codex"?"Codex":"Claude";
    const field=`${provider}_workers` as const, cap=`${provider}_weekly_limit` as const;
    return <div className="account-work" key={provider}>
     <div><label className="field__label" htmlFor={`${id}-${provider}-workers`}>{label} runners</label>
      <div className="worker-stepper">
       <button type="button" className="button button--secondary" aria-label={`Fewer ${label} workers`} disabled={settings[field]===0} onClick={()=>adjust(provider,(latest.current?.[field]??settings[field])-1)}><Minus size={18}/></button>
       <input id={`${id}-${provider}-workers`} className="input" type="number" min={0} max={settings[`${provider}_worker_limit`]} step={1} value={settings[field]} onChange={e=>{if(e.target.value!=="")adjust(provider,Number(e.target.value));}}/>
       <button type="button" className="button button--secondary" aria-label={`More ${label} workers`} disabled={settings[field]>=settings[`${provider}_worker_limit`]} onClick={()=>adjust(provider,(latest.current?.[field]??settings[field])+1)}><Plus size={18}/></button>
       <span className="muted small" role="status">{settings[field]===0?"Paused":`Up to ${settings[field]} working`}   max {settings[`${provider}_worker_limit`]}</span>
      </div>
     </div>
     <div className="account-work__cap"><label className="field__label" htmlFor={`${id}-${provider}-cap`}>Weekly usage ceiling <strong>{settings[cap]}%</strong></label>
      <input id={`${id}-${provider}-cap`} aria-label={`${label} weekly usage ceiling`} type="range" min={0} max={100} step={1} value={settings[cap]} onChange={e=>void change({[cap]:Number(e.target.value)})}/>
      <p className="muted small">Pause new calls at this percentage used. Resumes below the ceiling after the weekly reset, or when you raise it.</p>
     </div>
    </div>;
   })}
   <p className="muted small">Each account shares one login across its runners. Current calls finish when paused, so concurrent work can pass the ceiling slightly. Unknown or stale usage allows work, as requested. Up to 10 autonomous stories can be in flight across both accounts. Usage checks use scripts, not model tokens. {busy?"Saving ":""}</p>
  </div>}
 </section>;
}
