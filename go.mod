module github.com/wowsims/sod

go 1.21

require (
	github.com/google/go-cmp v0.6.0
	github.com/google/uuid v1.6.0
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/spf13/cobra v1.8.0
	github.com/tailscale/hujson v0.0.0-20221223112325-20486734a56a
	golang.org/x/exp v0.0.0-20240318143956-a85f2c67cd81
	google.golang.org/protobuf v1.33.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/sys v0.18.0 // indirect
)

// Local build: vanity import hosts are blocked in this sandbox, fetch from GitHub mirrors.
replace (
	golang.org/x/exp => github.com/golang/exp v0.0.0-20240318143956-a85f2c67cd81
	golang.org/x/sys => github.com/golang/sys v0.18.0
	google.golang.org/protobuf => github.com/protocolbuffers/protobuf-go v1.33.0
	gopkg.in/yaml.v3 => github.com/go-yaml/yaml/v3 v3.0.1
	gopkg.in/check.v1 => github.com/go-check/check v0.0.0-20201130134442-10cb98267c6c
)
