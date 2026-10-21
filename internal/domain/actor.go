package domain

// ActorKind says whether a change was made by a person, an agent, or the board.
type ActorKind string

// The actor kinds.
const (
	ActorHuman  ActorKind = "human"
	ActorAgent  ActorKind = "agent"
	ActorSystem ActorKind = "system"
)

// Actor is whoever performs a use case.
type Actor struct {
	Kind ActorKind `json:"kind"`
	ID   int64     `json:"id"`
	Name string    `json:"name"`
}

// SystemActor is used for changes the board makes on its own, such as moving a
// ticket back to refinement once every question is answered.
var SystemActor = Actor{Kind: ActorSystem, Name: "board"}

// IsHuman reports whether the actor is a signed-in person.
func (a Actor) IsHuman() bool {
	return a.Kind == ActorHuman
}

// IsAgent reports whether the actor authenticated with an agent token.
func (a Actor) IsAgent() bool {
	return a.Kind == ActorAgent
}
