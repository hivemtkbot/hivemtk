// 拆分自 sop.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

func NewWelcomeSOP() *CreateRequest {
	return &CreateRequest{

		Name:        "客户欢迎 SOP",
		Scenario:    "welcome",
		Description: "新客户接入时的标准欢迎流程（14 节点类型示范）",
		TriggerType: SOPTriggerAuto,
		SOPGraph: SOPGraph{
			Name:     "welcome_graph",
			Scenario: "welcome",
			Version:  "2.0",
			Entry:    "start",
			Exits:    []string{"end"},
			Nodes: []SOPNode{
				{ID: "start", Type: SOPNodeTypeStart, Name: "开始", Next: []string{"greeting"}},
				{
					ID:          "greeting",
					Type:        SOPNodeTypeGreeting,
					Name:        "问候",
					Description: "标准化客户问候",
					Prompt:      "您好，欢迎咨询，我是您的专属顾问",
					Next:        []string{"inquire"},
				},
				{
					ID:          "inquire",
					Type:        SOPNodeTypeInquire,
					Name:        "询问需求",
					Description: "了解客户核心诉求",
					Prompt:      "请问您想了解什么产品或服务？",
					Next:        []string{"end"},
				},
				{ID: "end", Type: SOPNodeTypeEnd, Name: "结束"},
			},
		},
	}
}
