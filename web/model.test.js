import test from "node:test";
import assert from "node:assert/strict";
import { emptyQuestion, validateDraft, buildRequest, sortProbabilities, formatAnswer, parseErrorBody } from "./model.js";

const draft = (q) => ({state:"ticket",questions:[q]});
test("buildRequest maps choice options and preserves null descriptions",()=>{
	const q={id:"dept",type:"choice",instructions:"pick",options:[{name:"returns",description:"refunds"},{name:"billing",description:null}]};
	assert.deepEqual(buildRequest(draft(q)),{state:"ticket",questions:{dept:{type:"choice",instructions:"pick",criteria:{returns:"refunds",billing:null}}}});
});
test("noul omits criteria unless advanced descriptions are set",()=>{
	const q={...emptyQuestion("noul","urgent"),instructions:"urgent?"};
	assert.equal(validateDraft(draft(q)).ok,true);
	assert.equal("criteria" in buildRequest(draft(q)).questions.urgent,false);
	q.trueDescription="needs attention";
	assert.deepEqual(buildRequest(draft(q)).questions.urgent.criteria,{true:"needs attention"});
});
test("score preserves level order",()=>{
	const q={id:"s",type:"score",instructions:"scale",levels:["Calm","Angry"]};
	assert.deepEqual(buildRequest(draft(q)).questions.s.criteria,q.levels);
});
test("empty state, ids and instructions rejected",()=>{
	const q={...emptyQuestion("noul","q"),instructions:"urgent?"};
	assert.equal(validateDraft({...draft(q),state:" \n "}).ok,false);
	assert.ok(validateDraft({...draft(q),state:" "}).errors.state);
	assert.equal(validateDraft(draft({...q,id:" "})).ok,false);
	assert.equal(validateDraft(draft({...q,instructions:" "})).ok,false);
	assert.equal(validateDraft({state:"ticket",questions:[]}).ok,false);
	assert.equal(validateDraft({state:"ticket",questions:[q,q]}).ok,false);
	assert.equal(validateDraft(draft({...q,type:"chat"})).ok,false);
});
for (const type of ["choice","score"]) {
	for (const n of [0,1,2,255,256]) test(`${type} count ${n}`,()=>{
		const q={...emptyQuestion(type,"q"),instructions:"pick"};
		if(type==="choice")q.options=Array.from({length:n},(_,i)=>({name:`n${i}`,description:null}));
		else q.levels=Array.from({length:n},(_,i)=>`L${i}`);
		assert.equal(validateDraft(draft(q)).ok,n>=2&&n<=255);
	});
}
test("choice empty/duplicate names and invalid descriptions rejected",()=>{
	for(const options of [[{name:""},{name:"b"}],[{name:"a"},{name:"a"}],[{name:"a",description:2},{name:"b"}]]) {
		assert.equal(validateDraft(draft({id:"q",type:"choice",instructions:"pick",options})).ok,false);
	}
});
test("score empty and non-string levels rejected",()=>{
	for(const levels of [["low",""],["low",null],["low",42]]) assert.equal(validateDraft(draft({id:"q",type:"score",instructions:"scale",levels})).ok,false);
});
test("special property names survive request construction",()=>{
	const q={id:"__proto__",type:"choice",instructions:"pick",options:[{name:"__proto__"},{name:"constructor"}]};
	const request=JSON.parse(JSON.stringify(buildRequest(draft(q))));
	assert.ok(Object.hasOwn(request.questions,"__proto__"));
	assert.ok(Object.hasOwn(request.questions.__proto__.criteria,"__proto__"));
});
test("probabilities sorted descending with stable ties",()=>{
	assert.deepEqual(sortProbabilities({a:.2,b:.6,c:.2}),[{name:"b",p:.6},{name:"a",p:.2},{name:"c",p:.2}]);
});
test("format all three answer types, retaining zero numeric values",()=>{
	const n=formatAnswer({type:"noul"},{noul:0});assert.equal(n.kind,"noul");assert.deepEqual(n.bars,[{name:"Yes",p:0},{name:"No",p:1}]);
	const c=formatAnswer({type:"choice"},{choice:"a",confidence:0,probabilities:{a:.7,b:.3}});assert.equal(c.headline,"a");assert.equal(c.confidence,0);assert.equal(c.bars.length,2);
	const s=formatAnswer({type:"score"},{score:1.44,legend:{0:"Calm",1:"Angry"},probabilities:{0:.2,1:.8}});assert.equal(s.headline,"1.44");assert.equal(s.modal,"1");assert.equal(s.legend[0],"Calm");
});
test("parse JSON and text errors",()=>{
	assert.equal(parseErrorBody('{"detail":"state too long"}'),"state too long");
	assert.equal(parseErrorBody("network down"),"network down");
	assert.match(parseErrorBody('{"detail":[{"msg":"invalid"}]}'),/invalid/);
});
