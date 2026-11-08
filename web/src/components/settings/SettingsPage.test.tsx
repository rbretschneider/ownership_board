import {act} from "react";
import {createRoot} from "react-dom/client";
import {expect,it,vi} from "vitest";
import {ApiContext} from "../../api/ApiContext";
import type {BoardApi} from "../../api/boardApi";
import type {AuthStatus} from "../../api/types";
import {repo as repoFixture} from "../../lib/flowFixture";
import {parseHash} from "../../lib/hashRoute";
import {ToastProvider} from "../toast/ToastProvider";
import {SettingsPage} from "./SettingsPage";
vi.mock("./AgentWorkSection",()=>({AgentWorkSection:()=>null}));

vi.mock("./TokensSection",()=>({TokensSection:()=>null}));
vi.mock("./AppearanceSection",()=>({AppearanceSection:()=>null}));
vi.mock("./AlertsSection",()=>({AlertsSection:()=>null}));
vi.mock("./AccountSection",()=>({AccountSection:()=>null}));
(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT:boolean}).IS_REACT_ACT_ENVIRONMENT=true;
it("scrolls and focuses the host section on direct and subsequent settings navigation",async()=>{
 const host=document.createElement("div"),root=createRoot(host);document.body.append(host);
 const scroll=vi.fn();const old=HTMLElement.prototype.scrollIntoView;HTMLElement.prototype.scrollIntoView=scroll;
 const api={buildHosts:vi.fn().mockResolvedValue([]),connections:vi.fn().mockResolvedValue([]),repos:vi.fn().mockResolvedValue([])} as unknown as BoardApi;
 const render=(hash:string)=>root.render(<ApiContext value={api}><ToastProvider><SettingsPage route={parseHash(hash)} status={{oidc:{label:"OIDC"}} as AuthStatus} onSignedOut={()=>{}} onChanged={()=>{}}/></ToastProvider></ApiContext>);
 try{
 await act(async()=>render("#/settings?section=build-hosts"));
 const section=host.querySelector("#build-hosts");expect(section).not.toBeNull();expect(document.activeElement).toBe(section);expect(scroll).toHaveBeenCalledTimes(1);
 await act(async()=>render("#/settings"));
 await act(async()=>render("#/settings?section=build-hosts"));
 expect(scroll).toHaveBeenCalledTimes(2);
 }finally{await act(async()=>root.unmount());host.remove();HTMLElement.prototype.scrollIntoView=old;}
});

it("saves and reloads shared patterns through repository settings",async()=>{
 const host=document.createElement("div");let root=createRoot(host);
 let stored={...repoFixture(3,"app"),overlap_ignore:["src/messages/Catalog.cpp"]};
 const changed=vi.fn();
 const api={buildHosts:vi.fn().mockResolvedValue([]),connections:vi.fn().mockResolvedValue([]),repos:vi.fn(async()=>[stored]),editRepo:vi.fn(async(_id:number,edit:{overlap_ignore:string[]})=>{stored={...stored,...edit};return stored;})};
 const render=()=>root.render(<ApiContext value={api as unknown as BoardApi}><ToastProvider><SettingsPage route={parseHash("#/settings")} status={{oidc:{label:"OIDC"}} as AuthStatus} onSignedOut={()=>{}} onChanged={changed}/></ToastProvider></ApiContext>);
 const click=async(text:string)=>act(async()=>{Array.from(host.querySelectorAll("button")).find(b=>b.textContent===text)!.click();});
 await act(async()=>render());
 expect(host.querySelector(".repo-row__controls")?.textContent).toContain("Files that never block parallel builds");
 await click("Edit shared files");
 await act(async()=>{const el=host.querySelector("textarea")!;Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,"value")!.set!.call(el,"src/*/Catalog.cpp");el.dispatchEvent(new Event("input",{bubbles:true}));});
 await click("Save");
 expect(api.editRepo).toHaveBeenCalledWith(3,{overlap_ignore:["src/*/Catalog.cpp"]});expect(changed).toHaveBeenCalledTimes(1);
 expect(host.querySelector(".repo-overlap-ignore")?.textContent).toContain("src/*/Catalog.cpp");
 await act(async()=>root.unmount());root=createRoot(host);await act(async()=>render());await click("Edit shared files");
 expect(host.querySelector("textarea")?.value).toBe("src/*/Catalog.cpp");await act(async()=>root.unmount());
});
