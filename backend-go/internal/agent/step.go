package agent

import "context"

// Step 是 Agent 工作流里的一个节点。
// 每个节点只做一件事：检索、生成、组装、校验或修正。
type Step interface {
	Name() string
	Run(ctx context.Context, state *State) error
}

// StepFunc 让普通函数也能快速变成 Step，适合写轻量步骤。
type StepFunc struct {
	name string
	run  func(ctx context.Context, state *State) error
}

func NewStepFunc(name string, run func(ctx context.Context, state *State) error) StepFunc {
	return StepFunc{name: name, run: run}
}

func (s StepFunc) Name() string {
	return s.name
}

func (s StepFunc) Run(ctx context.Context, state *State) error {
	return s.run(ctx, state)
}
