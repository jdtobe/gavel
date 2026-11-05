// Pure browser logic, shared with Node tests. No DOM or network dependencies.
export function emptyQuestion(type, id) {
	return {id, type, instructions:"", ...(type === "choice" ? {options:[{name:"",description:null},{name:"",description:null}]} : type === "score" ? {levels:["",""]} : {})};
}

const nonempty = value => typeof value === "string" && value.trim().length > 0;
export const isStateValue = value => nonempty(value) || (value !== null && typeof value === "object");
export function parseStateJSON(text) {
	try {
		const value = JSON.parse(text);
		return isStateValue(value) ? {ok:true,value,error:""} : {ok:false,value:null,error:"State must be a JSON string, object, or array."};
	} catch (error) { return {ok:false,value:null,error:error.message}; }
}
export function validateDraft(draft) {
	const errors = {};
	if (!isStateValue(draft.state) || draft.stateError) errors.state = draft.stateError || "State is required.";
	if (!draft.questions?.length) errors.questions = "Add at least one question.";
	const ids = new Set();
	for (const [i,q] of (draft.questions || []).entries()) {
		const key = `questions.${i}`;
		if (!nonempty(q.id) || ids.has(q.id)) errors[`${key}.id`] = "Use a unique, non-empty id.";
		ids.add(q.id);
		if (!["noul","choice","score"].includes(q.type)) errors[`${key}.type`] = "Choose a supported type.";
		if (!nonempty(q.instructions)) errors[`${key}.instructions`] = "Instructions are required.";
		if (q.type === "choice") {
			const options = q.options || [], names = new Set();
			if (options.length < 2 || options.length > 255) errors[`${key}.criteria`] = "Use 2–255 options.";
			for (const option of options) {
				if (!nonempty(option.name) || names.has(option.name)) errors[`${key}.criteria`] = "Option names must be unique and non-empty.";
				names.add(option.name);
				if (option.description != null && typeof option.description !== "string") errors[`${key}.criteria`] = "Descriptions must be text.";
			}
		}
		if (q.type === "score") {
			if (!Array.isArray(q.levels) || q.levels.length < 2 || q.levels.length > 255 || q.levels.some(x => !nonempty(x))) errors[`${key}.criteria`] = "Use 2–255 non-empty text levels.";
		}
		if (q.type === "noul") for (const name of ["trueDescription","falseDescription"]) {
			if (q[name] != null && typeof q[name] !== "string") errors[`${key}.criteria`] = "Descriptions must be text.";
		}
	}
	return {ok:Object.keys(errors).length === 0,errors};
}

export function validateQuestion(draft, question) {
	return validateDraft({...draft,questions:[question]});
}

export function changedQuestionIds(draft) {
	return draft.questions.filter(q=>q.dirty).map(q=>q.id);
}
function dirtyState(draft) {
	draft.stateRev=(draft.stateRev || 0)+1;
	for(const q of draft.questions) q.dirty=true;
}
export function editState(draft,text) {
	if (draft.stateMode === "json") {
		draft.stateText=text;
		const result=parseStateJSON(text);
		draft.stateError=result.error;
		if(result.ok) draft.state=result.value;
		dirtyState(draft);
	} else if(draft.state !== text) {
		draft.state=draft.stateText=text;
		draft.stateError="";
		dirtyState(draft);
	}
}
export function switchStateMode(draft,mode) {
	if(mode===draft.stateMode) return;
	if(mode==="json") {
		draft.stateText=JSON.stringify(draft.state,null,2);
	} else {
		const value=typeof draft.state === "string" ? draft.state : JSON.stringify(draft.state,null,2);
		if(value!==draft.state) {draft.state=value;dirtyState(draft);}
		draft.stateText=value;
	}
	draft.stateMode=mode;
	draft.stateError="";
}
export function editQuestion(draft,question,change) {
	change(question);
	question.rev=(question.rev || 0)+1;
	question.dirty=true;
}

export function buildRequest(draft, qids) {
	return {state:draft.state, questions:Object.fromEntries(draft.questions.filter(q=>!qids || qids.includes(q.id)).map(q => {
		const question = {type:q.type,instructions:q.instructions};
		if(q.type === "choice") question.criteria = Object.fromEntries(q.options.map(o => [o.name,o.description ?? null]));
		if(q.type === "score") question.criteria = [...q.levels];
		if(q.type === "noul" && (q.trueDescription || q.falseDescription)) {
			question.criteria = {};
			if(q.trueDescription) question.criteria.true = q.trueDescription;
			if(q.falseDescription) question.criteria.false = q.falseDescription;
		}
		return [q.id,question];
	}))};
}
export function sortProbabilities(probabilities = {}) {
	return Object.entries(probabilities).map(([name,p]) => ({name,p})).sort((a,b) => b.p-a.p);
}
export function formatAnswer(question, answer) {
	const kind = question.type;
	if(kind === "noul") return {kind,headline:`Yes · ${(answer.noul*100).toFixed(1)}%`,bars:[{name:"Yes",p:answer.noul},{name:"No",p:1-answer.noul}]};
	const bars = sortProbabilities(answer.probabilities);
	return {kind,headline:kind === "choice" ? answer.choice : Number(answer.score).toFixed(2),bars,confidence:answer.confidence, ...(kind === "score" ? {legend:answer.legend || {},modal:bars[0]?.name} : {})};
}
export function parseErrorBody(text) {
	try {const body=JSON.parse(text);return typeof body.detail === "string" ? body.detail : body.detail != null ? JSON.stringify(body.detail) : text;} catch {return text;}
}
