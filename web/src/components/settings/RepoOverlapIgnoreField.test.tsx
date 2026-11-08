import {act} from "react";
import {createRoot} from "react-dom/client";
import {expect,it,vi} from "vitest";
import {ApiContext} from "../../api/ApiContext";
import type {BoardApi} from "../../api/boardApi";
import type {Repo} from "../../api/types";
import {RepoOverlapIgnoreField} from "./RepoOverlapIgnoreField";

(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT:boolean}).IS_REACT_ACT_ENVIRONMENT=true;
const repo={id:3,full_name:"example/app",overlap_ignore:["src/messages/Catalog.cpp"]} as unknown as Repo;
async function mount(editRepo=vi.fn()){
 const host=document.createElement("div"),root=createRoot(host),onUpdated=vi.fn();
 await act(async()=>root.render(<ApiContext value={{editRepo} as unknown as BoardApi}><RepoOverlapIgnoreField repo={repo} onUpdated={onUpdated}/></ApiContext>));
 const click=async(text:string)=>act(async()=>{const button=Array.from(host.querySelectorAll("button")).find(b=>b.textContent===text);expect(button).toBeDefined();button!.click();});
 const change=async(value:string)=>act(async()=>{const el=host.querySelector("textarea")!;Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,"value")!.set!.call(el,value);el.dispatchEvent(new Event("input",{bubbles:true}));});
 return {host,click,change,onUpdated,unmount:()=>act(async()=>root.unmount())};
}
it("loads and saves trimmed nonblank patterns using the server response",async()=>{
 const saved={...repo,overlap_ignore:["src/*/Catalog.cpp","src/core/Errors.h"]};
 const api=vi.fn().mockResolvedValue(saved),v=await mount(api);
 expect(v.host.textContent).toContain("Files that never block parallel builds");
 await v.click("Edit shared files");expect(v.host.querySelector("textarea")?.value).toBe(repo.overlap_ignore.join("\n"));
 await v.change(" src/*/Catalog.cpp \n\n src/core/Errors.h \n");await v.click("Save");
 expect(api).toHaveBeenCalledWith(3,{overlap_ignore:saved.overlap_ignore});expect(v.onUpdated).toHaveBeenCalledWith(saved);
 expect(v.host.querySelector("textarea")).toBeNull();await v.unmount();
});
it("clears the list explicitly",async()=>{
 const api=vi.fn().mockResolvedValue({...repo,overlap_ignore:[]}),v=await mount(api);
 await v.click("Edit shared files");await v.change("\n ");await v.click("Save");expect(api).toHaveBeenCalledWith(3,{overlap_ignore:[]});await v.unmount();
});
it("validates limits, retains failed drafts and cancels",async()=>{
 const api=vi.fn().mockRejectedValue(new Error("Save failed")),v=await mount(api);
 await v.click("Edit shared files");
 for(const value of [Array(101).fill("src/file.go").join("\n"),"é".repeat(201)]) {
  await v.change(value);await v.click("Save");expect(api).not.toHaveBeenCalled();expect(v.host.querySelector('[role="alert"]')).not.toBeNull();
 }
 await v.change("é".repeat(200));await v.click("Save");expect(api).toHaveBeenCalledTimes(1);
 expect(v.host.textContent).toContain("Save failed");expect(v.host.querySelector("textarea")?.value).toBe("é".repeat(200));
 await v.click("Cancel");await v.click("Edit shared files");expect(v.host.querySelector("textarea")?.value).toBe(repo.overlap_ignore.join("\n"));await v.unmount();
});
it("disables controls and prevents duplicate saves",async()=>{
 let resolve:(value:Repo)=>void=()=>{};
 const api=vi.fn(()=>new Promise<Repo>(done=>{resolve=done;})),v=await mount(api);
 await v.click("Edit shared files");await v.click("Save");await v.click("Save");expect(api).toHaveBeenCalledTimes(1);
 expect(v.host.querySelector("fieldset")?.disabled).toBe(true);
 await act(async()=>resolve(repo));await v.unmount();
});
