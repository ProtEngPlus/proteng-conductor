package config

func GetSchema(schemaName string) string {
	if val, ok := Schemas[schemaName]; ok {
		return val
	}
	return `{}`
}

// Schemas is a map of service names to their respective options in JSON schemas format
// Further information on JSON schemas can be found at https://json-schema.org/
var Schemas = map[string]string{
	"blast": `
        {
            "$schema": "http://json-schema.org/draft-04/schema#",
            "type": "object",
            "properties": {
                "program": {
                    "type": "string"
                },
                "database": {
                    "type": "string"
                },
                "hitlist_size": {
                    "type": "integer"
                },
                "expect": {
                    "type": "number"
                },
                "perc_ident": {
                    "type": "number"
                },
                "random_state": {
                    "type": "integer"
                }
            },
            "required": [
                "program",
                "database",
                "hitlist_size",
                "expect",
                "perc_ident",
                "random_state"
            ]
        }`,
	"unirep": `
        {
            "$schema": "http://json-schema.org/draft-04/schema#",
            "type": "object",
            "properties": {
                "n_trials": {
                    "type": "integer"
                },
                "n_splits": {
                    "type": "integer"
                },
                "n_epochs_config_low": {
                    "type": "integer"
                },
                "n_epochs_config_high": {
                    "type": "integer"
                },
                "learning_rate_config_low": {
                    "type": "number"
                },
                "learning_rate_config_high": {
                    "type": "number"
                }
            },
            "required": [
                "n_trials",
                "n_splits",
                "n_epochs_config_low",
                "n_epochs_config_high",
                "learning_rate_config_low",
                "learning_rate_config_high"
            ]
        }`,
	"ridgecv": `
        {
            "$schema": "http://json-schema.org/draft-04/schema#",
            "type": "object",
            "properties": {
                "train_batch_sizes": {
                    "type": "array",
                    "items": {
                        "type": "integer"
                    }
                },
                "n_batch": {
                    "type": "integer"
                },
                "alpha": {
                    "type": "number"
                }
            },
            "required": [
                "train_batch_sizes",
                "n_batch",
                "alpha"
            ]
        }`,
	"mutation": `
        {
            "$schema": "http://json-schema.org/draft-04/schema#",
            "type": "object",
            "properties": {
                "temperature": {
                    "type": "number"
                },
                "num_iterations": {
                    "type": "integer"
                },
                "num_trajectories": {
                    "type": "integer"
                }
            },
            "required": [
                "temperature",
                "num_iterations",
                "num_trajectories"
            ]
        }`,
}
