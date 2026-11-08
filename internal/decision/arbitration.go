package decision

// 硬优先级仲裁（Q8）：多目标同时产生功率请求时，按固定硬优先级决定谁
// 胜出。优先级数值越小越硬，安全约束永远赢。这是 Site Scheduler 跨目标
// 协调的可解释、可追溯语义。

// GoalPriority 是调度目标的硬优先级。数值越小优先级越高。
type GoalPriority int

const (
	GoalSafety      GoalPriority = 0 // 安全（故障/越限硬停机）
	GoalAntiReverse GoalPriority = 1 // 防逆流
	GoalPeakShaving GoalPriority = 2 // 需量削峰
	GoalSchedule    GoalPriority = 3 // 计划曲线
	GoalPrice       GoalPriority = 4 // 电价套利
)

// Goal 是一个调度目标的一次请求。
type Goal struct {
	Priority  GoalPriority `json:"priority"`
	RequestKW float64      `json:"request_kw"`
}

// Arbitrate 硬优先级仲裁：返回优先级最高（数值最小）的目标请求。
// 冲突时高优先级目标胜出，低优先级目标让位；同优先级取先出现者。
func Arbitrate(goals []Goal) (Goal, bool) {
	if len(goals) == 0 {
		return Goal{}, false
	}
	best := goals[0]
	for _, goal := range goals[1:] {
		if goal.Priority < best.Priority {
			best = goal
		}
	}
	return best, true
}

// OrderByPriority 按硬优先级升序（最硬在前）返回目标列表，供顺序满足时
// 使用：高优先级目标先分配能力，剩余给低优先级。
func OrderByPriority(goals []Goal) []Goal {
	ordered := make([]Goal, len(goals))
	copy(ordered, goals)
	// 目标数量少（≤5 个定案目标），插入排序足够且稳定。
	for i := 1; i < len(ordered); i++ {
		for j := i; j > 0 && ordered[j].Priority < ordered[j-1].Priority; j-- {
			ordered[j], ordered[j-1] = ordered[j-1], ordered[j]
		}
	}
	return ordered
}
