package app

// Priority is how loudly a notification should interrupt.
type Priority int

// The priorities, matching ntfy's 1 to 5 scale.
const (
	PriorityLow     Priority = 2
	PriorityDefault Priority = 3
	PriorityHigh    Priority = 4
)

// Notification is a push message to the human.
type Notification struct {
	Title    string
	Message  string
	Priority Priority
	ClickURL string
	Tags     []string
	Actions  []NotificationAction
}

// NotificationAction is a button on a notification that opens a URL.
type NotificationAction struct {
	Label string
	URL   string
}
