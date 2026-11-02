import { emptyQuestion, validateDraft, buildRequest, formatAnswer, parseErrorBody } from "./model.js";

const $ = id => document.getElementById(id);
const el = (tag, text, className) => { const node=document.createElement(tag);if(text!=null)node.textContent=text;if(className)node.className=className;return node; };
let draft, ready=false, running=false, sequence=0;
let lastRequest=null,lastResponse=null;
const preset = () => ({state:"My shoes arrived three days late, in the wrong size. I also see two charges on my card. I need this sorted out today.",questions:[
	{id:"department",type:"choice",instructions:"Which team should handle this?",options:[{name:"returns",description:"Exchanges, refunds"},{name:"shipping",description:"Delivery"},{name:"billing",description:"Charges"}]},
	{id:"escalate",type:"noul",instructions:"Does this need urgent human attention?"},
	{id:"frustration",type:"score",instructions:"How frustrated is the customer?",levels:["Calm","Frustrated","Very angry"]}
]});

function validate() {
	const result=validateDraft(draft);
	$("state-error").textContent=result.errors.state||"";
	$("validation").textContent=Object.entries(result.errors).filter(([key])=>key!=="state").map(([,text])=>text).join(" ");
	$("characters").textContent=`${draft.state.length.toLocaleString()} characters`;
	$("run").disabled=!ready||running||!result.ok;
}
function field(parent,label,value,change,multiline=false) {
	const wrap=el("label",label), input=el(multiline?"textarea":"input");
	input.value=value??"";input.addEventListener("input",()=>{change(input.value);validate();});wrap.append(input);parent.append(wrap);return input;
}
function button(text,action) { const node=el("button",text);node.type="button";node.addEventListener("click",action);return node; }
function renderQuestions() {
	$("questions").replaceChildren();
	draft.questions.forEach((q,index)=>{
		const card=el("details",null,"card");card.open=Boolean(q.expanded);
		card.addEventListener("toggle",()=>{q.expanded=card.open;});
		const summary=el("summary",q.id||"New question");summary.append(el("span",q.type,"badge"));card.append(summary);
		field(card,"Question id",q.id,value=>{q.id=value;summary.firstChild.textContent=value||"New question";});
		field(card,"Instructions",q.instructions,value=>q.instructions=value,true);
		if(q.type==="noul") {
			const advanced=el("details");advanced.append(el("summary","Advanced: true / false descriptions"));
			field(advanced,"True",q.trueDescription,value=>q.trueDescription=value);field(advanced,"False",q.falseDescription,value=>q.falseDescription=value);card.append(advanced);
		} else {
			const values=q.type==="choice"?q.options:q.levels;
			values.forEach((value,i)=>{
				const row=el("div",null,`row ${q.type==="score"?"level":""}`);
				const name=el("input");name.value=q.type==="choice"?value.name:value;name.setAttribute("aria-label",`${q.id} ${q.type==="choice"?"option":"level"} ${i+1}`);
				name.addEventListener("input",()=>{if(q.type==="choice")value.name=name.value;else values[i]=name.value;validate();});row.append(name);
				if(q.type==="choice") {const description=el("input");description.value=value.description??"";description.placeholder="Description (optional)";description.setAttribute("aria-label",`${q.id} option ${i+1} description`);description.addEventListener("input",()=>{value.description=description.value||null;validate();});row.append(description);}
				const controls=el("div",null,"row-controls");
				for(const [text,delta] of [["↑",-1],["↓",1]]){const move=button(text,()=>{[values[i],values[i+delta]]=[values[i+delta],values[i]];renderQuestions();});move.disabled=i+delta<0||i+delta>=values.length;move.setAttribute("aria-label",`Move ${i+1} ${delta<0?"up":"down"}`);controls.append(move);}
				controls.append(button("×",()=>{values.splice(i,1);renderQuestions();}));row.append(controls);card.append(row);
			});
			const add=button(q.type==="choice"?"+ Option":"+ Level",()=>{values.push(q.type==="choice"?{name:"",description:null}:"");renderQuestions();});add.disabled=values.length>=255;card.append(add);
		}
		card.append(button("Delete question",()=>{if(card.open&&!window.confirm(`Delete ${q.id||"this question"}?`))return;draft.questions.splice(index,1);renderQuestions();}));
		$("questions").append(card);
	});
	validate();
}
function showRaw(){ $("raw-json").textContent=JSON.stringify({request:lastRequest,response:lastResponse},null,2); }
async function run() {
	if($("run").disabled)return;
	running=true;validate();$("error").hidden=true;
	const submitted=structuredClone(draft);
	lastRequest={...buildRequest(submitted),model:"kev-latest"};lastResponse=null;showRaw();
	$("answers").replaceChildren(...submitted.questions.map(q=>el("article",`${q.id} · evaluating…`,"card loading")));
	try {
		const response=await fetch("/api/systemone",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(lastRequest)});
		const text=await response.text();try{lastResponse=JSON.parse(text);}catch{lastResponse=text;}showRaw();
		if(!response.ok)throw new Error(parseErrorBody(text));
		$("answers").replaceChildren();
		for(const q of submitted.questions){
			const answer=lastResponse.answers?.[q.id], card=el("article",null,"card");card.append(el("h3",q.id));
			if(!answer){card.append(el("p","No answer returned."));$("answers").append(card);continue;}
			const formatted=formatAnswer(q,answer);card.append(el("p",formatted.headline,"answer-headline"));
			if(formatted.confidence!=null)card.append(el("p",`Confidence ${(formatted.confidence*100).toFixed(1)}%`,"confidence"));
			for(const {name,p} of formatted.bars){const row=el("div",null,`probability ${formatted.modal===name?"modal":""}`);row.append(el("span",formatted.legend?.[name]?`${name} · ${formatted.legend[name]}`:name),el("span",`${(p*100).toFixed(1)}%`));const bar=el("progress");bar.max=1;bar.value=p;bar.setAttribute("aria-label",name);row.append(bar);card.append(row);}
			$("answers").append(card);
		}
		$("usage").textContent=`${lastResponse.latency_ms??"–"} ms · ${lastResponse.usage?.input_tokens??"–"} input tokens · ${lastResponse.usage?.output_tokens??"–"} output tokens · ${response.headers.get("x-typesafe-request-id")||"local request"}`;
	} catch(err){$("error").textContent=err.message;$("error").hidden=false;$("answers").replaceChildren(el("p","The request failed. Your input is unchanged; try again.","empty"));}
	finally{running=false;validate();}
}
async function poll() {
	try{const response=await fetch("/api/status");if(!response.ok)throw new Error("Status unavailable");const status=await response.json();ready=status.phase==="ready";$("status").textContent=`${status.model} · ${status.backend||"local runtime"} · ${status.phase}${status.error?" · "+status.error:""}`;}
	catch{ready=false;$("status").textContent="Local runtime unavailable. Keep Gavel open in the terminal.";}
	validate();window.setTimeout(poll,2000);
}
$("state").addEventListener("input",()=>{draft.state=$("state").value;validate();});
$("add").addEventListener("click",()=>{let id;do{id=`q${++sequence}`;}while(draft.questions.some(q=>q.id===id));draft.questions.push({...emptyQuestion($("new-type").value,id),expanded:true});renderQuestions();});
$("preset").addEventListener("click",()=>{if(running)return;draft=preset();$("state").value=draft.state;renderQuestions();});
$("raw-toggle").addEventListener("click",()=>{$("raw").hidden=!$("raw").hidden;$("raw-toggle").setAttribute("aria-expanded",String(!$("raw").hidden));});
$("run").addEventListener("click",run);
document.addEventListener("keydown",event=>{if(event.metaKey&&event.key==="Enter"){event.preventDefault();run();}});
draft=preset();$("state").value=draft.state;renderQuestions();poll();
