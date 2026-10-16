# Utility Nodes

Utility nodes shape data, store it, render it, or make a run easier to understand.

24 node types.

**Debug**, **Log** and the sniffer on an edge all publish into the same live event stream
the editor's Debug tab reads.

| Node | Key | Ports | What it does |
|---|---|---|---|
| **Change** | `change` | in `input` · out `output` | Set or delete a value on msg, flow, or global scope |
| **Comment** | `comment` | in `input` · out `output` | Annotate a workflow without changing execution |
| **Date Format** | `date-format` | in `input` · out `output`, `error` | Parse and reformat a date or timestamp |
| **Debug** | `debug` | in `input` · out `output` | Emit the current message to the debug stream |
| **Delay** | `delay` | in `input` · out `output` | Pause workflow execution for a number of seconds |
| **Encode** | `encode` | in `input` · out `output`, `error` | Base64 encode or decode, or hash a value |
| **File** | `file` | in `input` · out `output`, `error` | Read or write a file in the workflow data directory |
| **HTTP Response** | `http-response` | in `input` · out `output` | Set the outgoing status code and headers on the message |
| **JSON Path** | `json-path` | in `input` · out `output` | Read a value from structured data with a simple JSONPath expression |
| **Junction** | `junction` | in `input` · out `output` | Create a simple routing point on the canvas |
| **KV Store** | `kv` | in `input` · out `output` | Read, write or delete a value in the workflow key-value store |
| **Link Out** | `link-out` | in `input` · out — | Store the current message for another workflow to consume |
| **Log** | `log` | in `input` · out `output` | Write a message to the workflow log stream |
| **Math** | `math` | in `input` · out `output`, `error` | Apply a math operation to a numeric value |
| **Merge Objects** | `merge-objects` | in `input` · out `output` | Merge multiple source objects into one output |
| **Metric Record** | `metric-record` | in `input` · out `output`, `error` | Persist a workflow metric sample from the current message |
| **Node Script** | `node-script` | in `input` · out `output`, `error` | Run custom JavaScript against the current workflow message |
| **Object** | `object` | in `input` · out `output`, `error` | Pick or omit fields, or map an object to a new shape |
| **Parse** | `parse` | in `input` · out `output`, `error` | Parse JSON, YAML, XML, CSV or HTML text into structured data |
| **Set Variable** | `set-variable` | in `input` · out `output` | Store a value on the flow context |
| **SQL Query** | `sql-query` | in `input` · out `output`, `error` | Run a read-only SQL query against the workflow database |
| **String Ops** | `string-ops` | in `input` · out `output`, `error` | Apply common string operations to a value |
| **Template** | `template` | in `input` · out `output`, `error` | Render a text template using values from the workflow message |
| **Webhook Response** | `webhook-response` | in `input` · out `output` | Prepare an HTTP response for a webhook-triggered flow |

## Configuration, node by node

### Change (`change`)

Set or delete a value on msg, flow, or global scope

- **Ports**: in: `input`; out: `output`
- **Fields**
  - Action (`action_type`): select, default `set`, one of `set`, `delete`
  - Scope (`scope`): select, default `msg`, one of `msg`, `flow`, `global`
  - Property (`property`): expression, required, default `payload`
  - Value (`value`): json; shown when `{"action_type":"set"}`

### Comment (`comment`)

Annotate a workflow without changing execution

- **Ports**: in: `input`; out: `output`
- **Fields**
  - Comment (`text`): textarea

### Date Format (`date-format`)

Parse and reformat a date or timestamp

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Property (`property`): expression, default `payload`
  - Output Format (`output_format`): select, default `rfc3339`, one of `rfc3339`, `rfc1123`, `date`, `datetime`, `time`, `date_eu`, `date_us`, `unix`
  - Timezone (`timezone`): timezone-select, default `UTC`

### Debug (`debug`)

Emit the current message to the debug stream

- **Ports**: in: `input`; out: `output`
- **Fields**
  - Property (`property`): expression, default `payload`
  - Show Full Message (`full_message`): toggle, default `false`

### Delay (`delay`)

Pause workflow execution for a number of seconds

- **Ports**: in: `input`; out: `output`
- **Fields**
  - Seconds (`seconds`): number, default `1`

### Encode (`encode`)

Base64 encode or decode, or hash a value

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Algorithm (`algorithm`): select, required, default `base64-encode`, one of `base64-encode`, `base64-decode`, `md5`, `sha1`, `sha256`, `sha512`
  - Property (`property`): expression, default `payload`
  - Output Property (`output_property`): expression, default `payload`; shown when `{"algorithm":["md5","sha1","sha256","sha512"]}`

### File (`file`)

Read or write a file in the workflow data directory

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Operation (`operation`): select, required, default `read`, one of `read`, `write`
  - Path (`path`): text, required
  - Encoding (`encoding`): select, default `utf-8`, one of `utf-8`, `base64`
  - Output Property (`output_property`): expression, default `payload`; shown when `{"operation":"read"}`
  - Property (`property`): expression, default `payload`; shown when `{"operation":"write"}`
  - Mode (`mode`): select, default `overwrite`, one of `overwrite`, `append`; shown when `{"operation":"write"}`

### HTTP Response (`http-response`)

Set the outgoing status code and headers on the message

- **Ports**: in: `input`; out: `output`
- **Fields**
  - Status Code (`status_code`): number, default `200`
  - Headers (`headers`): key-value

### JSON Path (`json-path`)

Read a value from structured data with a simple JSONPath expression

- **Ports**: in: `input`; out: `output`
- **Fields**
  - Expression (`expression`): text, required, default `$.payload`
  - Property (`property`): expression, default `payload`
  - Output Property (`output_property`): expression, default `payload`

### Junction (`junction`)

Create a simple routing point on the canvas

- **Ports**: in: `input`; out: `output`
- **Fields**: none

### KV Store (`kv`)

Read, write or delete a value in the workflow key-value store

- **Ports**: in: `input`; out: `output`
- **Fields**
  - Operation (`operation`): select, required, default `get`, one of `get`, `set`, `delete`
  - Key (`key`): text, required
  - Output Property (`output_property`): expression, default `payload`; shown when `{"operation":"get"}`
  - Default Value (`default_value`): text; shown when `{"operation":"get"}`
  - Property (`property`): expression, default `payload`; shown when `{"operation":"set"}`
  - TTL (seconds) (`ttl`): number, default `0`; shown when `{"operation":"set"}`

### Link Out (`link-out`)

Store the current message for another workflow to consume

- **Ports**: in: `input`; out: none
- **Fields**
  - Name (`name`): text, required

### Log (`log`)

Write a message to the workflow log stream

- **Ports**: in: `input`; out: `output`
- **Fields**
  - Level (`level`): select, default `info`, one of `info`, `warn`, `error`
  - Message (`message`): textarea

### Math (`math`)

Apply a math operation to a numeric value

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Property (`property`): expression, default `payload`
  - Operation (`operation`): select, default `add`, one of `add`, `subtract`, `multiply`, `divide`, `round`, `floor`, `ceil`, `abs`, `min`, `max`, `modulo`
  - Operand (`operand`): number, default `0`
  - Output Property (`output_property`): expression, default `payload`

### Merge Objects (`merge-objects`)

Merge multiple source objects into one output

- **Ports**: in: `input`; out: `output`
- **Fields**
  - Sources (`sources`): multiselect, required
  - Output Property (`output_property`): expression, default `payload`

### Metric Record (`metric-record`)

Persist a workflow metric sample from the current message

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Metric Name (`metric_name`): text, required
  - Property (`property`): expression, default `payload`
  - Metric Type (`metric_type`): select, default `gauge`, one of `gauge`, `counter`, `event`
  - Unit (`unit`): select, one of `ms`, `s`, `bytes`, `percent`, `count`
  - Labels (`labels`): key-value

### Node Script (`node-script`)

Run custom JavaScript against the current workflow message

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Code (`code`): code, required
  - Timeout (seconds) (`timeout`): number, default `10`

### Object (`object`)

Pick or omit fields, or map an object to a new shape

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Operation (`operation`): select, required, default `pick`, one of `pick`, `omit`, `map`
  - Property (`property`): expression, default `payload`
  - Fields (`fields`): multiselect; shown when `{"operation":["pick","omit"]}`
  - Mapping (`mapping`): key-value; shown when `{"operation":"map"}`

### Parse (`parse`)

Parse JSON, YAML, XML, CSV or HTML text into structured data

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Format (`format`): select, required, default `json`, one of `json`, `yaml`, `xml`, `csv`, `html`
  - Property (`property`): expression, default `payload`
  - Delimiter (`delimiter`): select, default `,`, one of `,`, `;`, `\t`, `\|`; shown when `{"format":"csv"}`
  - Has Headers (`has_headers`): toggle, default `true`; shown when `{"format":"csv"}`
  - Selector (`selector`): text, default `body`; shown when `{"format":"html"}`
  - Output Type (`output_type`): select, default `text`, one of `text`, `html`, `attribute`; shown when `{"format":"html"}`
  - Attribute (`attribute`): text; shown when `{"format":"html","output_type":"attribute"}`

### Set Variable (`set-variable`)

Store a value on the flow context

- **Ports**: in: `input`; out: `output`
- **Fields**
  - Variable Name (`name`): text, required
  - Value (`value`): json

### SQL Query (`sql-query`)

Run a read-only SQL query against the workflow database

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Query (`query`): textarea, required
  - Output Property (`output_property`): expression, default `payload`

### String Ops (`string-ops`)

Apply common string operations to a value

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Property (`property`): expression, default `payload`
  - Operation (`operation`): select, default `uppercase`, one of `uppercase`, `lowercase`, `trim`, `replace`, `split`, `join`, `substring`, `reverse`
  - Arg 1 (`arg1`): text
  - Arg 2 (`arg2`): text

### Template (`template`)

Render a text template using values from the workflow message

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Template (`template`): textarea, required
  - Output Property (`output_property`): expression, default `payload`

### Webhook Response (`webhook-response`)

Prepare an HTTP response for a webhook-triggered flow

- **Ports**: in: `input`; out: `output`
- **Fields**
  - Status Code (`status_code`): number, default `200`
  - Content Type (`content_type`): select, default `application/json`, one of `application/json`, `text/plain`, `text/html`, `application/xml`, `application/x-www-form-urlencoded`, `application/octet-stream`
  - Body (`body`): textarea

## Related

- [Node catalog](../README.md): all 90 node types, and how to read these pages
- [Execution model](../../execution/README.md): ports, emissions and error routing
- [The message contract](../../messages/README.md): what flows between these nodes
