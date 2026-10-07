package capability

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	archiveConversationEpisodeDescription = "Archive a reusable conclusion, decision, preference, diagnosis, or task outcome from this conversation. Do not archive ordinary chat."

	archiveConversationEpisodeSchema = `{"type":"object","additionalProperties":false,"required":["summary","topics","importance"],"properties":{"summary":{"type":"string","minLength":1,"maxLength":1000},"topics":{"type":"array","minItems":1,"maxItems":5,"items":{"type":"string","minLength":1,"maxLength":32}},"importance":{"type":"string","enum":["low","normal","high"]}}}`

	searchConversationHistoryDescription = "Search this user's selectively archived conversation history. Use it for prior decisions, diagnoses, preferences, or task outcomes."

	searchConversationHistorySchema = `{"type":"object","additionalProperties":false,"required":["query"],"properties":{"query":{"type":"string","minLength":1},"top_k":{"type":"integer","minimum":1,"maximum":5},"conversation_id":{"type":"string"}}}`

	coreMemoryAddDescription = "Save a user-requested, durable core preference. Use only when the user explicitly asks you to remember it."

	coreMemoryUpdateDescription = "Change one existing user core memory by memory_id. Use only when the user explicitly asks to change that remembered preference."

	coreMemoryDeleteDescription = "Forget one existing user core memory by memory_id. Use only when the user explicitly asks to forget it."

	retrieveKnowledgeDescription = "Search the user-authorized knowledge bases for evidence relevant to a question."

	createScheduledTaskDescription = "为一个未来的提醒、周期性汇总或条件监测创建定时任务草稿。" +
		"用户要求以后自动执行某事（例如「明天早上九点提醒我」「每天帮我查赛程，有新比赛就告诉我」）时使用。" +
		"时区由用户的设备决定，不要询问也不要猜测时区。" +
		"本工具只生成草稿，任务不会生效，用户需要在界面上确认后才会开始运行；" +
		"绝不要声称任务已经创建成功或已经开始运行。" +
		"缺少执行时间、频率或监测条件时必须先向用户澄清。" +
		"草稿中的定时任务默认可以使用联网搜索（web_search、web_fetch），不接入知识库。"

	createScheduledTaskSchema = `{"type":"object","additionalProperties":false,"required":["prompt","schedule","reportMode"],"properties":{` +
		`"name":{"type":"string","maxLength":60,"description":"任务标题，60 字以内，用于任务列表展示"},` +
		`"prompt":{"type":"string","minLength":1,"description":"每次触发时执行的完整指令，用第二人称描述要产出什么"},` +
		`"schedule":{"type":"object","additionalProperties":false,"required":["kind"],"properties":{` +
		`"kind":{"type":"string","enum":["once","interval","daily","weekly","monthly"]},` +
		`"atLocal":{"type":"string","description":"YYYY-MM-DD HH:MM 本地时间，kind=once 时必填，必须是将来的时间"},` +
		`"everyMinutes":{"type":"integer","minimum":1,"description":"间隔分钟数，kind=interval 时必填，从当前时刻开始计算"},` +
		`"localTime":{"type":"string","description":"HH:MM 本地时间，kind=daily/weekly/monthly 时必填"},` +
		`"weekday":{"type":"integer","minimum":0,"maximum":6,"description":"0=周日，1=周一，…，6=周六；kind=weekly 时必填"},` +
		`"monthDay":{"type":"integer","minimum":1,"maximum":31,"description":"每月几号；kind=monthly 时必填，该月没有这一天时跳过"}},` +
		`"allOf":[` +
		`{"if":{"properties":{"kind":{"const":"once"}}},"then":{"required":["atLocal"]}},` +
		`{"if":{"properties":{"kind":{"const":"interval"}}},"then":{"required":["everyMinutes"]}},` +
		`{"if":{"properties":{"kind":{"enum":["daily","weekly","monthly"]}}},"then":{"required":["localTime"]}},` +
		`{"if":{"properties":{"kind":{"const":"weekly"}}},"then":{"required":["weekday"]}},` +
		`{"if":{"properties":{"kind":{"const":"monthly"}}},"then":{"required":["monthDay"]}}]},` +
		`"reportMode":{"type":"string","enum":["always","on_condition"],"description":"always=每次触发都必须产出提醒或汇总；on_condition=只在条件成立时产出"},` +
		`"conditionKind":{"type":"string","enum":["none","event","state"],"description":"reportMode=on_condition 时必填；event=等待新事件，state=跟踪状态变化；reportMode=always 时必须为 none"}}}`

	listScheduledTasksDescription = "列出当前用户已保存的定时任务及其状态、计划与任务 ID。用户询问有哪些定时任务时必须使用本工具，不要凭记忆回答。暂停任务前先用本工具获取准确的 task_id。"

	listScheduledTasksSchema = `{"type":"object","properties":{}}`

	pauseScheduledTaskDescription = "暂停一个正在运行的定时任务，立即生效。" +
		"仅在用户明确要求暂停或停止某个任务时调用，且 task_id 必须来自 list_scheduled_tasks 返回的真实任务。" +
		"不要猜测任务 ID；如果无法确定用户指的是哪个任务，先向用户澄清。" +
		"已完成的任务不能暂停，也不要把它改回运行状态——恢复运行需要用户在任务管理页面确认。"

	pauseScheduledTaskSchema = `{"type":"object","required":["task_id"],"properties":{"task_id":{"type":"string","minLength":1,"description":"list_scheduled_tasks 返回的任务 ID"}}}`

	webSearchDescription = "Search the public web with Tavily."

	webSearchSchema = `{"type":"object","required":["query"],"properties":{"query":{"type":"string","minLength":1}}}`

	webFetchDescription = "Fetch readable text from up to three public URLs."

	webFetchSchema = `{"type":"object","required":["urls"],"properties":{"urls":{"type":"array","minItems":1,"maxItems":3,"items":{"type":"string"}}}}`
)
