// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package contract

// This file lets a plugin declare commands for the mcharbor CLI. Until it
// existed, every plugin capability in the product — every VM, container, Swarm
// service, pool and report — was reachable only through the interface and the
// API, because nothing in the process ever asked whether the CLI should reach
// it too.
//
// A command is a *declaration*, never a handler: it names a route the plugin
// already registered, and the CLI builds the request. That is what keeps the
// permission the route enforces and the permission the command advertises the
// same string, and what makes a route rename that forgets the CLI a compile
// error rather than a 404 an operator discovers.

// CLIProvider is implemented by plugins that contribute CLI commands. It is an
// optional interface the loader type-asserts, like TabProvider: a method on
// Plugin would break every implementation at compile time.
type CLIProvider interface {
	RegisterCLI(CLIRegistry) error
}

// CLIRegistry is the surface handed to RegisterCLI. Root must be called once,
// before any Add, because every command is addressed under it.
type CLIRegistry interface {
	Root(CLIRoot) error
	Add(CLICommand) error
}

// CLIRoot is the single top-level command group a plugin owns. The host refuses
// a name that collides with a core command or with another plugin's root, so a
// plugin can never shadow `container` or `audit`.
type CLIRoot struct {
	Name    string
	Short   string
	Aliases []string
}

// CLICommand is one command in a plugin's group. Route is the same string the
// plugin handed RegisterRoutes, and Needs must equal that route's declared
// permission — the host refuses the command otherwise.
type CLICommand struct {
	Path      []string
	Kind      CLIKind
	Method    string
	Route     string
	Args      []string
	Flags     []CLIFlag
	Body      CLIBody
	Needs     string
	Risk      CLIRisk
	Columns   []CLIColumn
	GuestKind string
	Short     string
	Long      string
	Example   string
	EnvScoped bool
}

// CLIFlag is one flag on a command. Target decides where the value goes, so the
// same flag vocabulary covers a query parameter, a JSON body field and a path
// placeholder without the plugin building a request itself.
type CLIFlag struct {
	Name     string
	Type     CLIFlagType
	Target   CLITarget
	Field    string
	Required bool
	Default  string
	Help     string
}

// CLIColumn is one column of a list or get rendering. Path is a dotted lookup
// into the response, and Format names one of the CLI's existing renderers.
type CLIColumn struct {
	Header string
	Path   string
	Format string
}

// CLIKind is how the CLI interacts with the route. The vocabulary is closed at
// seven because every one of them is proven by a route that exists today; a
// kind an older CLI does not recognise is skipped with a warning rather than
// failing the whole tree.
type CLIKind string

// The seven interaction kinds.
const (
	// CLIList renders an array or a paginated envelope through Columns.
	CLIList CLIKind = "list"
	// CLIGet renders one object.
	CLIGet CLIKind = "get"
	// CLIAction confirms by Risk, mutates, and reports in the past tense.
	CLIAction CLIKind = "action"
	// CLIText writes the response body to stdout verbatim, for a log tail.
	CLIText CLIKind = "text"
	// CLIUpload posts multipart/form-data and is exempt from --timeout,
	// because an image upload is minutes rather than seconds.
	CLIUpload CLIKind = "upload"
	// CLIDownload streams the body to a file or stdout.
	CLIDownload CLIKind = "download"
	// CLIAttach opens a console: a ticket POST against a core route, then a
	// WebSocket. It is the one kind whose Route is not the plugin's own.
	CLIAttach CLIKind = "attach"
)

// CLIBody says where a mutating command's request body comes from.
type CLIBody string

// The four body sources.
const (
	// BodyNone sends no body.
	BodyNone CLIBody = "none"
	// BodyFlags assembles the body from the flags whose target is the body.
	BodyFlags CLIBody = "flags"
	// BodyFile reads the body from --data @file, or from stdin for "-".
	BodyFile CLIBody = "file"
	// BodyBoth reads the file as the base and lets flags override fields in it.
	BodyBoth CLIBody = "both"
)

// CLIRisk maps onto the CLI's own confirmation rules, so a plugin's destructive
// command is guarded exactly as a core one is rather than by its own judgement.
type CLIRisk string

// The three risk levels.
const (
	// RiskSafe prompts for nothing.
	RiskSafe CLIRisk = "safe"
	// RiskDisruptive prompts on a terminal and is satisfied by --yes.
	RiskDisruptive CLIRisk = "disruptive"
	// RiskIrreversible refuses --yes and requires --confirm <target>.
	RiskIrreversible CLIRisk = "irreversible"
)

// CLIFlagType is a flag's value type, which decides how the CLI parses it.
type CLIFlagType string

// The flag value types.
const (
	// FlagString is the default.
	FlagString CLIFlagType = "string"
	// FlagInt is a whole number.
	FlagInt CLIFlagType = "int"
	// FlagBool is a switch.
	FlagBool CLIFlagType = "bool"
	// FlagDuration is a Go duration string.
	FlagDuration CLIFlagType = "duration"
	// FlagStringSlice is repeatable, or comma-separated.
	FlagStringSlice CLIFlagType = "stringSlice"
	// FlagFile names a local file; an upload command takes exactly one.
	FlagFile CLIFlagType = "file"
)

// CLITarget says which part of the request a flag's value lands in.
type CLITarget string

// The three flag targets.
const (
	// TargetQuery appends the value as a query parameter named by Field.
	TargetQuery CLITarget = "query"
	// TargetBody sets the value at Field, read as a dotted JSON path.
	TargetBody CLITarget = "body"
	// TargetPath fills the route placeholder named by Field.
	TargetPath CLITarget = "path"
)

// CLISpecVersion is the contract version the host serves. It is bumped only
// when the shape changes incompatibly, so a CLI that does not recognise the
// number can ignore the whole spec and say so in `doctor` rather than guess.
const CLISpecVersion = 1
