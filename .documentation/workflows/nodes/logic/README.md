# Logic Nodes

Logic nodes decide where a message goes, hold messages until something is ready, or bound
how fast a workflow runs.

13 node types.

Several of these are runtime-aware: **Range**, **Loop**, **Join** and **Aggregate** use
per-run buffers held by the executor rather than emitting one message per input.

| Node | Key | Ports | What it does |
|---|---|---|---|
| **Aggregate** | `aggregate` | in `input` · out `output` | Collect items for later aggregation in the executor |
| **Approval Gate** | `approval` | in `input` · out `approved`, `rejected`, `pending` | Route a workflow through an explicit approved or rejected gate |
| **Assert** | `assert` | in `input` · out `pass`, `fail` | Assert that a condition is true and branch on the result |
| **Condition** | `condition` | in `input` · out `true`, `false` | Route a message based on a condition |
| **Deduplicate** | `deduplicate` | in `input` · out `output`, `error` | Remove duplicate items from an array |
| **Filter** | `filter` | in `input` · out `pass`, `block` | Allow or block a message using a condition |
| **Join** | `join` | in `input` · out `output` | Join multiple incoming branches into a single message |
| **Loop** | `loop` | in `input` · out `done` | Iterate over an array from the current message |
| **Range** | `range` | in `input` · out `output`, `error` | Read an array property and attach range metadata |
| **Rate Limit** | `rate-limit` | in `input` · out `output` | Throttle workflow throughput with a simple rate policy |
| **Sort** | `sort` | in `input` · out `output`, `error` | Sort an array on the current message |
| **Switch** | `switch` | in `input` · out `case_0`, `default` | Route a message to dynamic case outputs |
| **Try Catch** | `try-catch` | in `input` · out `output`, `catch` | Guard downstream execution and route failures into a catch branch |

## Configuration, node by node

### Aggregate (`aggregate`)

Collect items for later aggregation in the executor

- **Ports**: in: `input`; out: `output`
- **Fields**
  - Group By (`group_by`): expression
  - Mode (`mode`): select, default `list`, one of `list`, `count`, `sum`

### Approval Gate (`approval`)

Route a workflow through an explicit approved or rejected gate

- **Ports**: in: `input`; out: `approved`, `rejected`, `pending`
- **Fields**
  - Decision (`decision`): select, required, default `pending`, one of `approved`, `rejected`, `pending`
  - Reason (`reason`): textarea

### Assert (`assert`)

Assert that a condition is true and branch on the result

- **Ports**: in: `input`; out: `pass`, `fail`
- **Fields**
  - Field (`field`): expression, required, default `payload`
  - Operator (`operator`): select, required, default `==`, one of `==`, `!=`, `contains`, `regex`, `is_empty`, `is_not_empty`
  - Value (`value`): text
  - Failure Message (`message`): text

### Condition (`condition`)

Route a message based on a condition

- **Ports**: in: `input`; out: `true`, `false`
- **Fields**
  - Field (`field`): expression, required, default `payload`
  - Operator (`operator`): select, required, default `==`, one of `==`, `!=`, `contains`, `regex`, `is_empty`, `is_not_empty`
  - Value (`value`): text

### Deduplicate (`deduplicate`)

Remove duplicate items from an array

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Property (`property`): expression, default `payload`
  - Unique By (`unique_by`): expression

### Filter (`filter`)

Allow or block a message using a condition

- **Ports**: in: `input`; out: `pass`, `block`
- **Fields**
  - Field (`field`): expression, required, default `payload`
  - Operator (`operator`): select, required, default `==`, one of `==`, `!=`, `contains`, `regex`, `is_empty`, `is_not_empty`
  - Value (`value`): text

### Join (`join`)

Join multiple incoming branches into a single message

- **Ports**: in: `input`; out: `output`
- **Fields**
  - Input Count (`input_count`): number, default `2`
  - Combine Mode (`combine_mode`): select, default `last`, one of `last`, `merge`, `array`

### Loop (`loop`)

Iterate over an array from the current message

- **Ports**: in: `input`; out: `done`
- **Fields**
  - Items Field (`items_field`): expression, default `payload`

### Range (`range`)

Read an array property and attach range metadata

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Property (`property`): expression, default `payload`

### Rate Limit (`rate-limit`)

Throttle workflow throughput with a simple rate policy

- **Ports**: in: `input`; out: `output`
- **Fields**
  - Limit (`limit`): number, default `10`
  - Window (seconds) (`window_seconds`): number, default `60`

### Sort (`sort`)

Sort an array on the current message

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Property (`property`): expression, default `payload`
  - Sort By (`sort_by`): expression
  - Direction (`direction`): select, default `asc`, one of `asc`, `desc`

### Switch (`switch`)

Route a message to dynamic case outputs

- **Ports**: in: `input`; out: `case_0`, `default`
- **Fields**
  - Property (`property`): expression, default `payload`
  - Check Type (`check_type`): select, default `value`, one of `value`, `type`, `regex`

### Try Catch (`try-catch`)

Guard downstream execution and route failures into a catch branch

- **Ports**: in: `input`; out: `output`, `catch`
- **Fields**
  - Error Property (`error_property`): expression, default `error`

## Related

- [Node catalog](../README.md): all 90 node types, and how to read these pages
- [Execution model](../../execution/README.md): ports, emissions and error routing
- [The message contract](../../messages/README.md): what flows between these nodes
