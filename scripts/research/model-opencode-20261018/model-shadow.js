// Lab-only live inference hook. Scores come from the frozen weights service.
export default async () => ({
  'tool.execute.before': async (input,output) => {
    const response=await fetch(process.env.AGENT_MODEL_ENDPOINT,{
      method:'POST',headers:{'Content-Type':'application/json','Authorization':'Bearer '+process.env.AGENT_MODEL_KEY},
      body:JSON.stringify({run_id:process.env.AGENT_MODEL_RUN,tool:input.tool,args:output.args,call_id:input.callID,session_id:input.sessionID}),
      signal:AbortSignal.timeout(15000),
    });
    if(!response.ok)throw new Error('Frozen model inference failed');
    const result=await response.json();
    if(result.mode!=='shadow_only'||!Array.isArray(result.scores)||!result.scores.length)throw new Error('Missing actual model scores');
    // A threshold hit is recorded by the server. No rule-generated decision.
  },
});
