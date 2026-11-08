import {act} from "react";
import {createRoot} from "react-dom/client";
import {expect,it,vi} from "vitest";
import {ApiContext} from "../../api/ApiContext";
import type {BoardApi} from "../../api/boardApi";
import type {Repo} from "../../api/types";
import {RepoBuildTargetsField} from "./RepoBuildTargetsField";

(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT:boolean}).IS_REACT_ACT_ENVIRONMENT=true;
const repo={id:3,full_name:"example/app",build_targets:[]} as unknown as Repo;
async function mount(editRepo=vi.fn(),value=repo){
 const host=document.createElement("div"),root=createRoot(host),onUpdated=vi.fn();
 await act(async()=>root.render(<ApiContext value={{editRepo} as unknown as BoardApi}><RepoBuildTargetsField repo={value} onUpdated={onUpdated}/></ApiContext>));
 const click=async(text:string)=>act(async()=>{const button=Array.from(host.querySelectorAll("button")).find(b=>b.textContent===text);expect(button).toBeDefined();button!.click();});
 const change=async(selector:string,value:string)=>act(async()=>{const el=host.querySelector(selector) as HTMLInputElement|HTMLSelectElement;expect(el).not.toBeNull();Object.getOwnPropertyDescriptor(el instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype,"value")!.set!.call(el,value);el.dispatchEvent(new Event(el instanceof HTMLSelectElement?"change":"input",{bubbles:true}));});
 return {host,root,click,change,onUpdated,unmount:()=>act(async()=>root.unmount())};
}
it("adds, edits and saves targets, accepting the canonical server response",async()=>{
 const saved={...repo,build_targets:[{os:"windows",preset:"script",capabilities:["gl","msvc"]}]};
 const api=vi.fn().mockResolvedValue(saved),v=await mount(api);
 expect(v.host.textContent).toContain("No build-host verification configured");
 await v.click("Edit build targets");await v.click("Add target");
 await v.change('[aria-label="Target 1 OS"]',"windows");
 await v.change('[aria-label="Target 1 preset"]',"script");
 await v.change('[aria-label="Target 1 capabilities"]',"msvc, gl, gl");
 await v.click("Save");
 expect(api).toHaveBeenCalledWith(3,{build_targets:[{os:"windows",preset:"script",capabilities:["msvc","gl","gl"]}]});
 expect(v.onUpdated).toHaveBeenCalledWith(saved);
 expect(v.host.querySelector('[aria-label="Target 1 OS"]')).toBeNull();
 await v.unmount();
});
it("retains failed drafts, validates tokens and duplicates, and cancels edits",async()=>{
 const api=vi.fn().mockRejectedValue(new Error("Save failed")),v=await mount(api);
 await v.click("Edit build targets");await v.click("Add target");
 await v.change('[aria-label="Target 1 capabilities"]',"Bad");await v.click("Save");
 expect(api).not.toHaveBeenCalled();expect(v.host.querySelector('[role="alert"]')).not.toBeNull();
 await v.change('[aria-label="Target 1 capabilities"]',"gl");await v.click("Save");
 expect(v.host.textContent).toContain("Save failed");expect(v.host.querySelector<HTMLInputElement>('input')?.value).toBe("gl");
 await v.click("Add target");await v.change('[aria-label="Target 2 capabilities"]',"gl");await v.click("Save");expect(api).toHaveBeenCalledTimes(1);
 await v.click("Cancel");expect(v.host.textContent).toContain("No build-host verification configured");
 await v.click("Edit build targets");expect(v.host.querySelector("select")).toBeNull();
 await v.unmount();
});
it("removes all targets and saves an explicit empty list",async()=>{
 const api=vi.fn().mockResolvedValue(repo),v=await mount(api,{...repo,build_targets:[{os:"linux",preset:"cmake",capabilities:[]}]});
 await v.click("Edit build targets");await v.click("Remove target 1");await v.click("Save");
 expect(api).toHaveBeenCalledWith(3,{build_targets:[]});await v.unmount();
});
it("prevents duplicate submissions while saving",async()=>{
 let resolve:(value:Repo)=>void=()=>{};
 const api=vi.fn(()=>new Promise<Repo>(done=>{resolve=done;})),v=await mount(api);
 await v.click("Edit build targets");await v.click("Save");await v.click("Save");expect(api).toHaveBeenCalledTimes(1);
 await act(async()=>resolve(repo));await v.unmount();
});
