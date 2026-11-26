#!/usr/bin/env node
// Finite counterexample models for the AD review. No production code is imported.
// Model results are not runtime tests, proof of implementation, or probabilities.
function verify(){
  const permutations=a=>a.length===0?[[]]:a.flatMap((v,i)=>permutations(a.filter((_,j)=>j!==i)).map(p=>[v,...p]));
  // Abstract four-step interleavings. U is a per-kind revocation or threshold
  // increase, not a total gate closure. C is the worker's broad-gate check.
  const schedules=permutations(["Q","C","U","A"]).filter(p=>p.indexOf("Q")<p.indexOf("C")&&p.indexOf("C")<p.indexOf("A")&&p.indexOf("Q")<p.indexOf("U"));
  const stale=schedules.map(trace=>{
    let generation=0,queued=-1,checked=false,oldApplied=false,targetApplied=false,staleOld=false,staleTarget=false;
    for(const step of trace){
      if(step==="Q")queued=generation;
      if(step==="C")checked=true;
      if(step==="U")generation++;
      if(step==="A"){
        oldApplied=checked;
        targetApplied=checked&&queued===generation;
        staleOld=oldApplied&&queued!==generation;
        staleTarget=targetApplied&&queued!==generation;
      }
    }
    return {trace:trace.join("→"),oldApplied,targetApplied,staleOld,staleTarget};
  });
  // A binary key cannot encode which independent actor owns the restriction.
  // Compare a hypothetical delete-on-revoke implementation, not a claim that
  // the current feedback-disable handler deletes map entries.
  const ownership=[];
  for(const manual of [false,true])for(const automatic of [false,true]){
    const bit=manual||automatic;
    const deleteOnAutoRevoke=automatic?false:bit;
    const sourceAwareAfterRevoke=manual;
    ownership.push({manual,automatic,deleteOnAutoRevoke,sourceAwareAfterRevoke,violatesManual:manual&&!deleteOnAutoRevoke});
  }
  // Synthetic CPU-local samples: sequence 2 reached userspace but was
  // intentionally filtered. No kernel reserve failure occurred.
  const input=[{seq:1,drop:false},{seq:2,drop:true},{seq:3,drop:false}];
  const gaps=s=>s.slice(1).reduce((n,x,i)=>n+Math.max(0,x.seq-s[i].seq-1),0);
  const loss={input,postFilterGap:gaps(input.filter(x=>!x.drop)),preFilterGap:gaps(input),intentionalFiltered:input.filter(x=>x.drop).length};
  // The ideal target classifies every path/inode operation result. This is
  // only the truth table for the documented success contract.
  const mapResults=[];
  for(const pathOK of [false,true])for(const inodeOK of [false,true]){
    mapResults.push({pathOK,inodeOK,pathOnlySuccess:pathOK,bothRequiredSuccess:pathOK&&inodeOK,partial:pathOK&&!inodeOK});
  }
  const assertions={
    staleOldWitnesses:stale.filter(x=>x.staleOld).length===2,
    noStaleTarget:stale.every(x=>!x.staleTarget),
    ownershipCounterexample:ownership.filter(x=>x.violatesManual).length===1,
    lossCounterexample:loss.postFilterGap===1&&loss.preFilterGap===0&&loss.intentionalFiltered===1,
    partialMapCounterexample:mapResults.filter(x=>x.partial).length===1
  };
  return {scope:"abstract model only, no real kernel or runtime calls",stale,ownership,loss,mapResults,assertions};
}
const result = verify();
console.log(JSON.stringify(result, null, 2));
if (!Object.values(result.assertions).every(Boolean)) process.exitCode = 1;
