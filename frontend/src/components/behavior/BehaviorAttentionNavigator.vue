<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { LeftOutlined, RightOutlined, AimOutlined } from "@ant-design/icons-vue";
import { webLocale } from "../../i18n";
import type { AgentEvent } from "../../composables/dashboard/dashboardConstants";
import { attentionTimeline, groupAttention, type AttentionOrder } from "../../composables/behavior/behaviorAttention";
const props=defineProps<{events:readonly AgentEvent[];now:number;windowMs:number;selectedKey?:string;viewedKeys:readonly string[]}>();
const emit=defineEmits<{inspect:[event:AgentEvent]}>();
const text=(zh:string,en:string)=>webLocale.value==="en-US"?en:zh;
const order=ref<AttentionOrder>("risk");
const page=ref(0);
const groups=computed(()=>groupAttention(props.events,props.now,props.windowMs,order.value));
const pages=computed(()=>Math.max(1,Math.ceil(groups.value.length/4)));
const visibleGroups=computed(()=>groups.value.slice(page.value*4,page.value*4+4));
const buckets=computed(()=>attentionTimeline(props.events,props.now,props.windowMs));
const peak=computed(()=>Math.max(1,...buckets.value.map(bucket=>bucket.count)));
const recordCount=computed(()=>groups.value.reduce((sum,group)=>sum+group.members.length,0));
const critical=computed(()=>groups.value.filter(group=>(group.maxRisk??0)>=80).length);
const latest=computed(()=>Math.max(0,...groups.value.map(group=>group.lastAt)));
const age=(at:number)=>{
 if(!at)return text("时间未知","Unknown time");
 const seconds=Math.max(0,Math.floor((props.now-at)/1000));
 return seconds<2?text("刚刚","Just now"):seconds<60?text(seconds+"秒前",seconds+"s ago"):text(Math.floor(seconds/60)+"分钟前",Math.floor(seconds/60)+"m ago");
};
const clock=(at:number)=>new Date(at).toLocaleTimeString(webLocale.value,{hour12:false});
function inspectNext():void {
 const index=groups.value.findIndex(group=>group.members.some(event=>event.key===props.selectedKey));
 const next=groups.value[(index+1)%groups.value.length];if(next)emit("inspect",next.representative);
}
watch([groups,()=>props.selectedKey],()=>{
 const index=groups.value.findIndex(group=>group.members.some(event=>event.key===props.selectedKey));
 if(index>=0)page.value=Math.floor(index/4);
 else page.value=Math.min(page.value,pages.value-1);
});
watch(order,()=>{page.value=0;});
</script>
<template>
 <section class="attention-navigator" :aria-label="text('关注事件与时间定位','Attention events and time navigation')">
  <header><div><h2>{{text('优先核查','Priority review')}}</h2><span>{{groups.length}} {{text('组','groups')}} · {{recordCount}} {{text('条记录','records')}}<template v-if="critical"> · {{critical}} {{text('组高风险','high-risk groups')}}</template></span></div><div class="attention-actions"><select v-model="order" :aria-label="text('关注事件排序','Attention event ordering')"><option value="risk">{{text('风险优先','Risk first')}}</option><option value="recent">{{text('最近优先','Most recent')}}</option></select><button :disabled="!groups.length" @click="inspectNext"><AimOutlined />{{text('下一项','Next item')}}</button></div></header>
  <div class="attention-summary"><span>{{text('最近关注','Latest attention')}}：{{latest?age(latest):text('窗口内无记录','No records in window')}}</span><span>{{text('重复行为折叠；查看状态不表示已处理。','Repeats folded; viewed does not mean resolved.')}}</span></div>
  <div v-if="groups.length" class="attention-cards">
   <button v-for="group in visibleGroups" :key="group.key" class="attention-card" :class="{critical:(group.maxRisk??0)>=80,selected:group.members.some(event=>event.key===selectedKey)}" :data-risk="group.maxRisk??'unknown'" :data-event-key="group.representative.key" :data-event-id="group.representative.eventId || group.representative.key" @click="emit('inspect',group.representative)">
    <span class="attention-card-title"><span>{{group.representative.comm}} · {{group.representative.type}}</span><strong>{{group.maxRisk??'—'}}</strong></span>
    <span class="attention-card-target">{{group.representative.netEndpoint||group.representative.path||group.representative.decision||'—'}}</span>
    <span class="attention-card-meta"><span>{{age(group.lastAt)}} · {{group.members.length}} {{text('条','records')}}</span><span>{{viewedKeys.includes(group.representative.key)?text('证据已查看','Evidence viewed'):text('未查看','Not viewed')}}</span></span>
   </button>
  </div>
  <div v-else class="attention-empty">{{text('当前范围没有需关注记录；空窗口不代表系统安全。','No attention records in this scope. An empty window does not establish safety.')}}</div>
  <div class="attention-timebar"><span>{{text('关注时间轴','Attention timeline')}}</span><div class="attention-buckets" role="group" :aria-label="text('点击时间段定位风险证据','Choose a time bucket to inspect evidence')"><button v-for="(bucket,index) in buckets" :key="index" class="attention-bucket" :disabled="!bucket.representative" :aria-label="clock(bucket.start)+' – '+clock(bucket.end)+' · '+bucket.count+' '+text('条需关注记录','attention records')" :title="clock(bucket.start)+' – '+clock(bucket.end)+' · '+bucket.count" @click="bucket.representative&&emit('inspect',bucket.representative)"><i :style="{height:Math.max(2,bucket.count/peak*26)+'px'}"/><span class="sr-only">{{bucket.count}}</span></button></div><span>{{text('现在','Now')}}</span></div>
  <footer><span>{{text('分数优先，同分按最近发生；缺少分数保留原始证据。','Scores first, then recency; unscored evidence remains available.')}}</span><div><button :disabled="page===0" :aria-label="text('上一组关注记录','Previous attention page')" @click="page--"><LeftOutlined /></button><code>{{page+1}} / {{pages}}</code><button :disabled="page>=pages-1" :aria-label="text('下一组关注记录','Next attention page')" @click="page++"><RightOutlined /></button></div></footer>
 </section>
</template>
<style scoped>
.attention-navigator{min-width:0;padding:14px 16px;background:var(--surface-2);border:.5px solid var(--border);border-radius:12px;color:var(--text-primary)}
header,header>div,.attention-actions,.attention-summary,footer,footer>div{display:flex;align-items:center;gap:10px}header,footer,.attention-summary{justify-content:space-between}header>div:first-child{flex-wrap:wrap}h2{font-size:16px;font-weight:500;margin:0}header span,.attention-summary,footer{font-size:11px;color:var(--text-muted)}button,select{font:11px var(--font-sans);background:var(--surface-2);color:var(--text-secondary);border:.5px solid var(--border);border-radius:8px;padding:6px 8px;min-height:32px}button{cursor:pointer}button:disabled{opacity:.4;cursor:default}button:focus-visible,select:focus-visible{outline:2px solid var(--text-accent);outline-offset:2px}.attention-actions{flex-shrink:0}.attention-actions button{display:flex;gap:5px;align-items:center}.attention-summary{margin:9px 0;gap:12px;flex-wrap:wrap}.attention-cards{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:7px;min-height:140px}.attention-card{text-align:left;padding:9px 11px;display:grid;gap:4px;border-left:2px solid var(--text-warning);min-width:0}.attention-card.critical{border-left-width:4px}.attention-card.selected{background:var(--bg-accent);border-color:var(--text-accent)}.attention-card-title{display:flex;justify-content:space-between;align-items:center;gap:8px}.attention-card-title>span{overflow:hidden;white-space:nowrap;text-overflow:ellipsis;font:12px var(--font-mono);color:var(--text-primary)}.attention-card-title strong{font:500 18px var(--font-mono);color:var(--text-warning)}.attention-card-target{color:var(--text-muted);font:11px var(--font-mono);overflow:hidden;white-space:nowrap;text-overflow:ellipsis}.attention-card-meta{display:flex;justify-content:space-between;gap:8px;color:var(--text-muted);font-size:11px}.attention-empty{min-height:140px;display:flex;align-items:center;justify-content:center;color:var(--text-muted);font-size:13px;padding:12px;box-sizing:border-box}.attention-timebar{display:flex;align-items:center;gap:9px;margin-top:9px;font-size:11px;color:var(--text-muted)}.attention-timebar>span{flex-shrink:0}.attention-buckets{display:flex;gap:3px;flex:1;overflow-x:auto;min-width:0;padding:3px}.attention-bucket{display:flex;align-items:flex-end;justify-content:center;flex:1;min-width:12px;height:32px;min-height:32px;padding:2px;border-radius:3px;border:0;background:var(--surface-0)}.attention-bucket i{display:block;width:100%;background:var(--text-warning)}.attention-bucket:disabled{opacity:.35}.attention-navigator footer{margin-top:7px;gap:14px}.attention-navigator footer>span{line-height:1.6}.attention-navigator footer button{min-height:26px;padding:3px 6px}.sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}@container(max-width:700px){header{align-items:flex-start;flex-wrap:wrap}.attention-cards{grid-template-columns:minmax(0,1fr)}.attention-summary>span:last-child{display:none}.attention-navigator footer{flex-wrap:wrap}.attention-bucket{min-width:20px}}@media(forced-colors:active){.attention-card{border:1px solid ButtonText}.attention-bucket i{background:Mark}}
</style>
