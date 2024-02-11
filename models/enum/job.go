package enum

type JobState string

const (
	JobStateCreated   JobState = "CREATED"
	JobStatePending   JobState = "PENDING"
	JobStateOnGoing   JobState = "ONGOING"
	JobStateCompleted JobState = "COMPLETED"
	JobStateFailed    JobState = "FAILED"
)

type MutationState string

const (
	MutationStatePending   MutationState = "PENDING"
	MutationStateOnGoing   MutationState = "ONGOING"
	MutationStateCompleted MutationState = "COMPLETED"
	MutationStateFailed    MutationState = "FAILED"
)
