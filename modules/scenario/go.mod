module system1/scenario

go 1.25.0

require (
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.7.6
	github.com/labstack/echo/v4 v4.15.1
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2
	github.com/umu-art/System1/api/build/s1-workflow-validator-client v0.0.0
	gopkg.in/yaml.v3 v3.0.1
	s1_workflow_api v0.0.0
	s1_workflow_validator_api v0.0.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/rogpeppe/go-internal v1.16.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	golang.org/x/time v0.14.0 // indirect
)

replace s1_workflow_api => ../../api/build/s1-workflow-api

replace s1_workflow_validator_api => ../../api/build/s1-workflow-validator-api

replace github.com/umu-art/System1/api/build/s1-workflow-validator-client => ../../api/build/s1-workflow-validator-client
