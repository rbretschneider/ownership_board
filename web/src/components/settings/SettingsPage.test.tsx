import {act} from "react";
import {createRoot} from "react-dom/client";
import {expect,it,vi} from "vitest";
import {ApiContext} from "../../api/ApiContext";
import type {BoardApi} from "../../api/boardApi";
import type {AuthStatus} from "../../api/types";
import {parseHash} from "../../lib/hashRoute";
import {ToastProvider} from "../toast/ToastProvider";
import {SettingsPage} from "./SettingsPage";
vi.mock("./AgentWorkSection",()=>({AgentWorkSection:()=>null}));
vi.mock("./RepositoriesSection",()=>({RepositoriesSection:()=>null}));
vi.mock("./TokensSection",()=>({TokensSection:()=>null}));
vi.mock("./AppearanceSection",()=>({AppearanceSection:()=>null}));
vi.mock("./AlertsSection",()=>({AlertsSection:()=>null}));
vi.mock("./AccountSection",()=>({AccountSection:()=>null}));
(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT:boolean}).IS_REACT_ACT_ENVIRONMENT=true;
it("scrolls and focuses the host section on direct and subsequent settings navigation",async()=>{
 const host=document.createElement("div"),root=createRoot(host);document.body.append(host);
 const scroll=vi.fn();const old=HTMLElement.prototype.scrollIntoView;HTMLElement.prototype.scrollIntoView=scroll;
 const api={buildHosts:vi.fn().mockResolvedValue([])} as unknown as BoardApi;
 const render=(hash:string)=>root.render(<ApiContext value={api}><ToastProvider><SettingsPage route={parseHash(hash)} status={{oidc:{label:"OIDC"}} as AuthStatus} onSignedOut={()=>{}} onChanged={()=>{}}/></ToastProvider></ApiContext>);
 try{
 await act(async()=>render("#/settings?section=build-hosts"));
 const section=host.querySelector("#build-hosts");expect(section).not.toBeNull();expect(document.activeElement).toBe(section);expect(scroll).toHaveBeenCalledTimes(1);
 await act(async()=>render("#/settings"));
 await act(async()=>render("#/settings?section=build-hosts"));
 expect(scroll).toHaveBeenCalledTimes(2);
 }finally{await act(async()=>root.unmount());host.remove();HTMLElement.prototype.scrollIntoView=old;}
});
