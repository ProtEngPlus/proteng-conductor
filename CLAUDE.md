# proteng-conductor

Pipeline orchestrator of ProtEngPlus (Go, Gin, MongoDB, RabbitMQ). It stores jobs, mutations and query results, publishes the current stage to the topic exchange `logs_topic`, reads results from `job_status_event`, and picks the next stage. It is called only through proteng-bff and does no JWT checks itself.

**This repo is public.** Never add IPs, hostnames of the deploy machines, NodePorts, runbooks or any credential, including in comments and commit messages. Those live in the private repos manual-guides-2023 and devops-infra.

## Commands

- `make check` before finishing any change: gofmt check, `go vet`, `go test`. CI currently runs only gofmt and vet; tests join CI with PR #124, so run them locally.
- `make fmt`, `make run` (needs RabbitMQ and MongoDB from `make -C ../manual-guides-2023 infra-up`), `make test-race`, `make build`.
- `make mocks` after changing an interface that has `//go:generate mockgen`. Use `github.com/golang/mock/mockgen@v1.6.0` to match `go.mod`; `go.uber.org/mock` generates different imports.
- On Windows a CRLF working tree makes `gofmt -l` list every file; that is a checkout problem, not a code problem.

## Code layout

- `internal/conductor/conductor.go`: `RunJob`, `RunMutation`, `Orchestrate`, `OrchestrateJob`, `updateJobData`, `getNextStage`, `sendJobToPipelineComponent`, `sendJobStatusNotificationEmail`. Messages are defined in `internal/conductor/model.go`.
- Stage ids 0 to 3 are `query`, `evotune`, `fittop`, `mutation`; the routing key is `<stage>.<tool>`, for example `query.mmseqs2`.
- `internal/rabbitmq/consumer` (reconnect, graceful shutdown) and `internal/rabbitmq/publisher` (`PublishWithTopic`, `PublishDefaultExchange`).
- `apis/`, `models/`, `repositories/` (mocks in `repositories/mock_repository`), `storage/` for GCS.

## Things that break

- `updateJobData` applies a result when the stage matches, without checking that the job is still `ONGOING`; `Orchestrate` checks the state only after the update. A late result can move a job that was set to `FAILED`. Keep this in mind when touching result handling (related to BLB1).
- ML queues are exclusive and publishes use `mandatory=false`, so a message to a stage with no connected consumer is dropped with no error.
- `getNextStage` decides `ONGOING` versus `PENDING` from `RunType == "auto"`; manual jobs wait for the user between stages.
- Deploying while jobs run can lose in-flight results.
- New Go functions need a test in a `_test.go` file of the same package, in the same commit.
- A push to `dev` deploys dev and a push to `main` deploys production, even for docs-only changes.

## Team workflow

Issue first with a commit plan, branch from `dev`, Conventional Commits in English with one topic per commit, PR into `dev` using the template in Thai, no emoji anywhere. Full rules: `CONTRIBUTING.md` of manual-guides-2023.
