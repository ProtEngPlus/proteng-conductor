# proteng-conductor

Gin + MongoDB job orchestrator: the center of ProtEngPlus. Manages job/pipeline state, consumes stage-completion events from RabbitMQ, and dispatches the next pipeline stage to the [proteng-kubeflow](https://github.com/ProtEngPlus/proteng-kubeflow) microservices. Called by [proteng-bff](https://github.com/ProtEngPlus/proteng-bff).

See [SETUP.md](./SETUP.md) to get it running locally, and [CONTRIBUTING.md](./CONTRIBUTING.md) for commit conventions and pre-commit hooks.
