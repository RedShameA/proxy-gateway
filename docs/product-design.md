# Proxy Gateway Product Design

## Overview

Proxy Gateway is a single-node Docker service that provides a web console, management API, HTTP Proxy, and SOCKS5 Proxy through one Single External Port. Users import Subscriptions or add individual Nodes, create Access Profiles in the web console, and then use an Access Profile's Profile Identifier plus Proxy Credential passwords in proxy clients.

The service uses sing-box as the Node protocol engine, not as the product configuration model. Product behavior is modeled around Nodes, Node Observations, Access Profiles, Proxy Credentials, Profile Evaluation, and Proxy Request Logs.

## Goals

- Provide a web console for managing Subscriptions, manual Nodes, Access Profiles, Proxy Credentials, observations, and request logs.
- Expose one external Docker port for Web UI, API, HTTP Proxy, and SOCKS5 Proxy.
- Support HTTP Proxy and SOCKS5 TCP CONNECT with an Access Profile's Profile Identifier and Proxy Credential passwords.
- Import Nodes from remote or local Subscriptions and manually added single Nodes.
- Maintain background Profile Evaluation so proxy requests do not block on speed tests.
- Support fixed-node, fastest-node, random-node, and two-hop chain Access Profiles, with country filtering expressed as Candidate Filters rather than separate Access Profile types.
- Persist configuration and runtime state across container restarts.

## Non-Goals

- Do not provide a full sing-box configuration editor.
- Do not expose arbitrary sing-box route, DNS, inbound, rule set, or experimental API configuration.
- Do not support multi-hop chains beyond two hops in the MVP.
- Do not support SOCKS5 UDP ASSOCIATE or BIND in the MVP.
- Do not record request bodies, response bodies, or packet captures.
- Do not support multi-instance coordination in the MVP.

## Core Concepts

### Node

A Node is a proxy server configuration. It may come from a Subscription or be manually added. Nodes are deduplicated by normalized configuration, not by display name or source.

If the same Node appears in multiple Subscriptions or is manually added again, the service keeps one Node and attaches multiple Node Sources to it.

Subscription refresh uses successful snapshot replacement semantics:

- when a Subscription refresh succeeds, that Subscription's Node Sources are replaced by the newly parsed result
- Nodes that no longer appear in that Subscription lose that Subscription as a Node Source
- a deduplicated Node is deleted only when it has no remaining Node Sources
- if refresh fetch or parsing fails, the previous successful Subscription snapshot remains active
- Manual Node Import sources are not changed by Subscription refresh
- refresh result summaries include added, retained, updated, removed, ignored, skipped, and errored counts
- upstream strategy groups and routing constructs are ignored Subscription Entries, not skipped Nodes

Node deletion is source-based:

- Nodes imported from a Subscription cannot be deleted one by one
- deleting a Subscription removes that Subscription's Node Sources
- deleting a Manual Node Import removes only that manual Node Source
- if the same deduplicated Node still has another Subscription or Manual Node Import source, the Node remains
- the deduplicated Node is deleted only after its last Node Source is removed

Disabling a Node is different from deleting a Node Source. A Disabled Node remains visible with its sources, but it is intentionally excluded from proxy path selection. Subscription refresh does not automatically re-enable Disabled Nodes.

### Node Observation

Node Observation is runtime information measured about a Node:

- whether it is currently usable
- last error
- observed exit IP
- Egress Country
- latency samples
- last successful probe time
- last failed probe time

Node Observations are shared across all Node Sources for the same deduplicated Node.

Node Observation discovers Egress Country in two steps:

1. Fetch an Egress IP Probe through the Node to learn the observed egress IP.
2. Resolve that IP against a local GeoIP Database.

The Egress IP Probe is only responsible for returning the observed IP address. It is not the source of truth for country or region classification.

Node Observation does not accept a user-provided Test URL. A manual "observe now" action in the Web Console triggers the same observation flow used by Background Maintenance: usability, egress IP, Egress Country, and basic latency. This keeps Node Observation separate from target-specific Profile Evaluation.

The default Egress IP Probe follows Resin's behavior: fetch `https://cloudflare.com/cdn-cgi/trace` through the Node and parse the `ip=` field from the response. The measured request latency may be recorded as the Node's basic latency for `cloudflare.com`. The Egress IP Probe endpoint should be configurable for deployments that cannot or do not want to depend on Cloudflare, but it is not shown as a normal Test URL field in the Nodes view.

Observation Latency is separate from Path Evaluation Latency. Observation Latency is a basic Node-level signal captured during Node Observation. It is the time for the gateway to use the Node to reach the Egress IP Probe and receive a response; it is not a ping to the Node or just the time to connect to the Node. The Web Console label is `探测耗时`.

Path Evaluation Latency is measured by Profile Evaluation for a specific Access Profile, Test URL, or chain mode. The Web Console must not present it as the same metric as Observation Latency.

The GeoIP Database uses Resin-compatible defaults:

- format: MaxMind-compatible `mmdb`
- default filename: `country.mmdb`
- default source: latest GitHub Release from `MetaCubeX/meta-rules-dat`
- storage: under `/data` so Docker deployments keep it across restarts
- startup behavior: load existing database if present; if missing or stale, download in the background
- update behavior: scheduled update by cron-style Maintenance Schedule, default daily at `07:00` local time
- replacement behavior: download to a temporary file, verify release-provided SHA256 when available, then atomically replace the old database
- failure behavior: keep using the last successfully loaded database; if none is available, Egress Country remains unknown without blocking Node Observation

### Access Profile

An Access Profile is a named proxy access configuration created in the web console. It defines how a Proxy Path should be selected for Target Connections.

Each Access Profile has one unique Profile Identifier. Proxy clients do not send complex selection rules on every request; they authenticate with the Access Profile's Profile Identifier and one Proxy Credential password. The Chinese UI presents Access Profile as 访问策略 and Profile Identifier as 策略标识. The Web Console may prefill a Profile Identifier from the Access Profile name, but uniqueness conflicts must block saving instead of silently changing the identifier. Changing a Profile Identifier invalidates previously copied proxy URLs that used the old identifier.

### Proxy Credential

A Proxy Credential is used by HTTP Proxy and SOCKS5 Proxy clients. It contains:

- plaintext password
- required remark
- enabled/disabled flag
- last used time

One Access Profile may have multiple Proxy Credentials so passwords can be issued, identified by remark, and revoked per client or location while sharing the same Profile Identifier.

Proxy Credential remarks are required and 1 to 64 characters. When creating a new Proxy Credential, the Web Console prefills `凭证 N`, where `N` is the current number of existing Proxy Credentials under that Access Profile plus one. For example, if two credentials currently exist, the default remark is `凭证 3`.

Proxy Credential remarks are not required to be unique within an Access Profile. They are admin-facing labels only and do not participate in authentication, revocation, or log lookup identity.

Proxy Credential passwords are 6 to 32 characters. New credentials default to an 8-character generated password. Passwords only allow the URL-safe character set `A-Z`, `a-z`, `0-9`, `-`, and `_`, so complete proxy URLs do not depend on percent-encoding password characters.

Proxy Credential passwords must be unique within one Access Profile, because HTTP Proxy and SOCKS5 authentication identify a credential by the Profile Identifier plus password pair. The same password may be reused under a different Access Profile.

The Web Console generates the default 8-character password when the create-credential form opens and places it in the editable password input. The backend validates length, character set, and uniqueness within the Access Profile, but it does not generate the default password for this flow.

After successful proxy authentication, the service updates the matched Proxy Credential's `last_used_at`, throttled to at most once every 60 seconds per credential. Proxy Request Logs still record individual proxy requests; `last_used_at` is only a summary field for credential lists and details.

### Proxy Path

A Proxy Path is either:

- a single Node
- a two-hop chain: `Front Node -> Exit Node`

The Exit Node determines the Egress Country.

### Proxy Path Summary

`ProxyPathSummary` is the shared API shape for current served path, Best Observed Proxy Path, Profile Evaluation events, switch events, and request-log path snapshots.

For a single-node Proxy Path, `ProxyPathSummary` includes:

- path kind
- Node id
- Node display name
- protocol
- Egress IP
- Egress Country
- Node Observation Latency, when available
- Path Evaluation Latency
- latency kind

For a chain Proxy Path, `ProxyPathSummary` includes:

- path kind
- Front Node summary
- Exit Node summary
- final Egress Country
- Chain Evaluation Mode
- Node Observation Latency for the Front and Exit Node summaries, when available
- Path Evaluation Latency
- latency kind

Country display must follow the Egress Country dictionary: the Web Console shows Chinese country name plus ISO code, and shows Unknown Egress Country as `未知`. API enum and sentinel values must not be shown directly to users.

For chain Proxy Paths, Path Evaluation Latency must match the Chain Evaluation Mode:

- `chain_link`: `latency_kind` is `chain_link`; the Web Console label is `链路耗时`; it measures the `Front Node -> Exit Node` chain setup.
- `end_to_end`: `latency_kind` is `end_to_end`; the Web Console label is `整链耗时`; it measures the full `Front Node -> Exit Node -> Test URL` result.

For single-node Profile Evaluation on Fastest Profiles, `latency_kind` is `end_to_end`; the Web Console label is `访问耗时`; it measures `Node -> Test URL`. Fixed Node Profiles and Random Profiles do not run Profile Evaluation. For those profiles, the Web Console may show the Node summary's Observation Latency, labeled `探测耗时`; this fallback must not be represented as a `latency_kind` value.

The Web Console must show the Chinese latency label and must not display the raw `latency_kind` value.

## Entrypoint

The service exposes one external port.

The Entrypoint dispatches traffic by protocol shape:

- Web UI and API: normal HTTP requests such as `/`, `/assets/*`, `/api/*`
- HTTP Proxy: `CONNECT` requests and absolute-form HTTP proxy requests
- SOCKS5 Proxy: SOCKS5 handshake bytes

HTTP Proxy authentication uses `Proxy-Authorization` with the Profile Identifier as the username and a Proxy Credential password as the password. SOCKS5 authentication uses the same username/password pair. Web UI and API use Admin Credential authentication and must not share Proxy Credentials.

Proxy clients receive a generic authentication failure for missing Profile Identifier, wrong password, and disabled Proxy Credential cases. Client-facing proxy errors must not reveal whether an Access Profile or Proxy Credential exists.

Proxy Credential copy URLs are generated by the backend, not assembled in the browser. The generated HTTP Proxy and SOCKS5 Proxy URLs use the Access Profile's Profile Identifier as the username and the Proxy Credential plaintext password as the password. The fixed formats are `http://<Profile Identifier>:<password>@<Proxy Access Address>` and `socks5://<Profile Identifier>:<password>@<Proxy Access Address>`, with no path, query, or fragment. Their host and optional port come from the configured Proxy Access Address. If Proxy Access Address is blank, the backend may fall back to the current management API request Host, but it must not trust `X-Forwarded-Host`, `X-Forwarded-Proto`, or other forwarding headers for this fallback. When the fallback Host is an IPv6 address, the generated URL must normalize it to `[IPv6]` or `[IPv6]:port`, never a bare IPv6 URL. Reverse-proxy deployments that need correct external copy URLs should configure Proxy Access Address explicitly. Proxy Access Address is trimmed before saving; an empty trimmed value means not configured. Proxy Access Address stores a domain/IPv4 `host` or `host:port`, or an IPv6 `[addr]` or `[addr]:port`; it must not include a URL scheme, path, query, or fragment. If an explicitly configured Proxy Access Address has no port, the backend uses it as written and does not append the service port. Only blank-configuration fallback uses the port already present in the request Host. `localhost`, `127.0.0.1`, and local IPv6 addresses are allowed and do not require a special Web Console warning.

## Authentication

There are two credential types:

- Admin Credential: logs into the web console and management API.
- Proxy Credential: authenticates proxy clients after the Profile Identifier selects an Access Profile.

They are separate and cannot be reused across surfaces.

On first startup, if no Admin Credential exists, the service must force an initial setup flow or accept an explicit initialization secret from deployment configuration. Admin Credential passwords and tokens are stored only as hashes. Proxy Credential passwords are stored directly as plaintext so the Web Console can keep showing complete proxy URLs for copying.

## Subscription and Node Management

### Subscription Sources

The MVP supports:

- remote Subscriptions fetched from `http://` or `https://`
- local Subscriptions pasted into the web console or API
- Manual Node Import

### Subscription Formats

The MVP supports:

- sing-box JSON: `{"outbounds":[...]}` or a raw outbound array
- Clash YAML/JSON: `proxies`
- common URI lines: `ss://`, `vmess://`, `vless://`, `trojan://`, `hysteria2://`, `socks://`, `http://`
- base64-wrapped URI text

Unsupported entries are skipped with parse warnings. Import only creates Nodes for entries that can be converted into valid normalized sing-box outbound JSON. A Subscription refresh should not delete existing usable Nodes until the new content has been fetched and parsed successfully.

Subscription import should expose a skipped-entry summary, for example unsupported functional outbounds, missing required protocol fields, or unsupported protocol options grouped by reason.

Subscription Import Summary should be understandable without knowing Clash or sing-box internals. The Subscriptions view should group skipped entries by user-facing reason and allow expanding to raw details.

Recommended skipped-entry groups:

- strategy groups: selector, urltest, fallback, load-balance, or other functional groups that are not dialable Nodes
- unsupported protocols
- missing required fields
- invalid format
- duplicate Nodes
- disabled source

Example summary text: `Strategy groups: 11 skipped because they are not dialable Nodes`. Expanded details may show raw names such as `x11`.

### Node Protocol Scope

The service supports sing-box remote proxy outbound protocols as Nodes:

- `socks`
- `http`
- `shadowsocks`
- `vmess`
- `trojan`
- `naive`
- `wireguard`
- `hysteria`
- `shadowtls`
- `vless`
- `tuic`
- `hysteria2`
- `anytls`
- `tor`
- `ssh`

Deprecated sing-box remote proxy outbounds are supported on a best-effort basis when the embedded sing-box version still exposes them. They are not guaranteed to remain supported after sing-box removes them.

sing-box functional outbounds such as `block`, `dns`, `selector`, and `urltest` are not imported as Nodes. `selector` and `urltest` overlap with Access Profile and Profile Evaluation, so they stay outside the Node model.

`direct` remains a special product Node for local direct dialing and smoke tests.

Subscription groups such as Clash `proxy-groups`, sing-box `selector`, and sing-box `urltest` are not converted into Access Profiles. Subscription import only creates Nodes from remote proxy entries; users define Access Profiles in the web console.

### Candidate Filter

Access Profiles may restrict candidate Nodes with a Candidate Filter:

- `node_source_mode`: choose which Node Sources are eligible: all Nodes, manual Nodes only, Subscription Nodes only, or selected Subscriptions
- `source_ids`: only consider Nodes from selected Node Sources when `node_source_mode` is selected Subscriptions
- `egress_country_mode`: choose whether `egress_countries` is an include set or an exclude set
- `egress_countries`: selected Egress Countries for the include/exclude filter; this uses the real observed egress IP country, not country words in the Node name, and may include an explicit Unknown Egress Country sentinel
- `name_include_regex`: include Nodes matching display name or source label
- `name_exclude_regex`: exclude Nodes matching display name or source label

The MVP does not include a full expression language.

Disabled Nodes never match Candidate Filters for automatic selection. Fastest, Random, Front Node selection, and chain evaluation must all avoid Disabled Nodes.

Unknown Egress Country is an explicit dictionary option. The API sentinel is `__unknown__`, but the Web Console must display it as `未知`. In include mode, unknown-country Nodes match only when that option is selected. In exclude mode, unknown-country Nodes are excluded only when that option is selected; otherwise they remain eligible. Creating or updating an automatic-selection Access Profile with an Egress Country Candidate Filter should trigger Node Observation for relevant Nodes whose Egress Country is unknown.

The Web Console should label country filters as "Egress Country" or "出口国家", not just "Country" or "国家", so admins do not confuse observed exit location with subscription naming. Country filter controls use a full country dictionary, include an explicit "未知" option backed by `__unknown__`, put common countries first, and are presented as multi-selects. Selection still matches only the real Egress Country discovered by Node Observation or the explicit Unknown Egress Country state; choosing countries with no matching Nodes causes filtered Access Profiles to have no candidates.

Egress Country filtering is a general Candidate Filter condition for automatic-selection Access Profiles, not a separate Access Profile type. Fastest Profile and Random Profile apply Candidate Filter to single-node candidates. Chain Access Profile uses the same `candidate_filter` field, but in chain mode it applies only to Front Node candidates; any Egress Country condition there applies to the Front Node's observed Egress Country and must be labeled as Front Node Egress Country or 前置节点出口国家 in the Web Console. Fixed Node Profile does not use Candidate Filter; it only displays the selected Node's observed Egress Country.

## Access Profile Types

Access Profile type values are limited to `fixed_node`, `fastest`, `random`, and `chain`. Egress Country filtering and chain evaluation behavior do not create separate type values.

Chain Access Profile has a `chain_evaluation_mode`:

- `chain_link`: the UI label is `最快前置`; select the fastest Front Node by Chain Link Evaluation against an Exit Node selected from the Exit Node Set.
- `end_to_end`: the UI label is `整链最快`; select the Front Node by End-to-End Evaluation through `Front Node -> Exit Node -> Test URL`.

The default `chain_evaluation_mode` is `end_to_end`.
`chain_link` is valid only when `exit_node_ids` contains exactly one Node id. If `exit_node_ids` contains more than one Node id, `chain_evaluation_mode` must be `end_to_end`.
When an admin has selected `chain_link` and then changes the Exit Node Set from one Node to multiple Nodes, the Web Console must switch the mode to `end_to_end` and show a clear hint such as "多出口节点仅支持整链最快". The backend must still reject invalid combinations.

| Original Need | Access Profile Type | Behavior |
| --- | --- | --- |
| Choose a specific node | Fixed Node Profile | Always uses the selected Node if usable. It does not run Profile Evaluation. |
| Provide Test URL and choose fastest node | Fastest Profile | Uses End-to-End Evaluation over candidate single-node Proxy Paths after applying Candidate Filter. |
| Randomly choose a usable node | Random Profile | Applies Candidate Filter and picks a random usable Node per Target Connection. |
| Fixed Exit Node set, auto-select Front Node | Chain Access Profile | Uses the selected Chain Evaluation Mode, fixed Exit Node set, and Candidate Filter to choose a two-hop Proxy Path. |

## Profile Evaluation

Profile Evaluation runs in the background and maintains each Access Profile's current usable Proxy Path or candidate pool.

Proxy requests must not trigger unbounded or synchronous evaluation. They use the latest Profile Evaluation result.

A Profile Evaluation Cycle starts from a snapshot of one Access Profile's current configuration. When the cycle starts, the previous Best Observed Proxy Path is cleared and the service starts calculating Best Observed again from successful results in the new cycle.

Within a cycle, Profile Evaluation evaluates the Proxy Paths that match the configuration snapshot and Candidate Filter at cycle start. For chain mode, that means the eligible `Front Node x Exit Node` complete path set from that snapshot. The cycle should run that matching set without product-level candidate or pair cropping.

If the Access Profile configuration changes while a cycle is running, the running cycle is superseded: its results must not overwrite the newer configuration, and a new cycle is queued for the new configuration snapshot. If new Nodes, Node Observations, or source changes appear after a cycle has started, they are handled by a later cycle rather than inserted into the current cycle.

Only one Profile Evaluation Cycle may run for a given Access Profile at a time. If a schedule tick, manual evaluate action, Node Observation completion, or other trigger requests evaluation while a cycle for the same Access Profile is already running, the trigger is merged into a pending rerun flag. When the current cycle finishes, the service immediately starts one new cycle from the latest Access Profile configuration and latest Node state if a pending rerun exists.

### Test URL Semantics

End-to-End Evaluation uses HTTP GET to the Test URL. Test URL is only used by Profile Evaluation; it is not used by Node Observation and does not determine Egress Country.

Test URL supports both `http://` and `https://` URLs. The Web Console should treat `https://` as the default scheme when a user enters a bare hostname such as `example.com`.

The system default Test URL is `https://www.gstatic.com/generate_204`. System Settings may override this default, and each Access Profile that uses target-specific fastest selection may override it again.

Fixed Node Profiles and Random Profiles do not use Test URL and do not run Profile Evaluation. Their serving readiness is derived from configured Node references, Candidate Filter where applicable, Disabled Node state, and Node Observation.

Default success rule:

- HTTP status `200-399` counts as success.
- Other status codes count as failure unless configured otherwise.
- Response body reads are capped, for example at `64KB`, so large responses do not dominate measurements.

Ranking should prioritize recent success rate first and latency second. Latency should use a smoothed metric such as EMA or a recent-window percentile, not only the last sample.

### Chain Link Evaluation

Chain Link Evaluation applies to Chain Access Profile when `chain_evaluation_mode` is `chain_link`.

It must test whether traffic can go through the Front Node and complete the Exit Node's real proxy protocol handshake. A simple TCP port check to the Exit Node is insufficient.

Chain Link Evaluation does not fetch a Test URL. It measures the chain segment needed to establish the Exit Node through the Front Node.

For chain Access Profiles, the Exit Node set is explicitly selected by the admin and may contain one or more Nodes. Candidate Filter does not apply to the Exit Node set. Each selected Exit Node determines the final Egress Country for paths that use it. Front Nodes are selected automatically by applying Candidate Filter to Front Node candidates only. Front Node candidates must exclude Disabled Nodes, unusable Nodes, every selected Exit Node, any deduplicated Node identical to a selected Exit Node, and Nodes whose protocol configuration cannot be dialed by sing-box. Egress Country conditions in Candidate Filter apply to the Front Node's observed Egress Country only; they do not filter or change the fixed Exit Node set or final Egress Country.

The configuration field for the Exit Node Set is `exit_node_ids`. It must contain at least one Node id. All other Candidate Filter fields, including source, name, and Egress Country filters, apply only to Front Node candidates in chain mode.

### End-to-End Chain Evaluation

Chain Access Profile with `chain_evaluation_mode` set to `end_to_end` evaluates:

```text
Front Node -> Exit Node -> Test URL
```

When the Exit Node Set contains multiple Nodes, End-to-End Chain Evaluation evaluates eligible `Front Node x Exit Node` combinations as complete Proxy Paths. It must not first choose the best Exit Node and then choose the best Front Node as a separate stage.

If the Chain Access Profile has no current usable Proxy Path, the first successful `Front Node -> Exit Node -> Test URL` result becomes current immediately. Evaluation continues in the background for other eligible combinations and maintains the Best Observed Proxy Path separately from the currently served Proxy Path. A later complete Proxy Path replaces the current path only when it satisfies Switching Tolerance, or the current path fails, becomes stale, or becomes unavailable.

### Stability

Fastest-style Access Profiles should not wait for all possible candidates before they become usable, and they should not switch Proxy Paths on every minor measurement change.

When no current usable Proxy Path exists, the first successful eligible path becomes current immediately. Background Profile Evaluation continues checking other eligible candidates or chain combinations and maintains the Best Observed Proxy Path separately from the current Proxy Path. The current Proxy Path is then kept until:

- the current path fails
- a different path clears the relative improvement threshold
- a different path clears the absolute latency improvement threshold
- the current path becomes stale or unavailable

Random Profile is different: it must choose a truly random currently usable Node for each new Target Connection after applying Candidate Filter. It does not use sticky selection in the MVP and is not affected by fastest-profile stability thresholds.

Switching Tolerance controls how often fastest-style Access Profiles replace a usable path:

- do not switch while the current Proxy Path remains usable for minor latency changes
- switch immediately when a new Proxy Path has recent successful evaluations and clears either the configured relative percentage improvement or the configured absolute latency improvement
- switch away immediately when the current Proxy Path has failed the latest `2` evaluations
- allow switching when the current Proxy Path has no successful evaluation for more than `3` evaluation cycles
- manual "evaluate now" triggers Profile Evaluation but follows the same Switching Tolerance rules and does not force a path switch by itself
- the Web Console may expose an explicit "switch to Best Observed Proxy Path" action when a valid Best Observed Proxy Path exists and differs from the current served Proxy Path

Best Observed Proxy Path is valid only when:

- it still matches the current Access Profile configuration and Candidate Filter
- its latest evaluation succeeded
- it has no newer failure record
- it belongs to the current running Profile Evaluation Cycle, or to the most recently completed cycle when no newer cycle has started

Best Observed Proxy Path does not have a separate time-based freshness setting. When a new Profile Evaluation Cycle starts, the previous Best Observed Proxy Path is cleared and the service starts calculating it again from successful results in the new cycle.

Default Switching Tolerance values:

- relative improvement threshold: `20%`
- absolute latency improvement threshold: `100ms`

Random Profile candidate rules:

- only currently usable Nodes are eligible
- Candidate Filter must match, including Egress Country include/exclude rules when configured
- every HTTP CONNECT, SOCKS5 CONNECT, or HTTP absolute-form upstream connection gets a fresh random choice
- existing Target Connections never switch mid-connection
- if no candidate is available, proxy requests fail quickly

## Profile Evaluation Concurrency

Profile Evaluation uses background worker and schedule controls. These controls regulate worker load and cadence; they do not decide which eligible candidates are allowed to be evaluated.

Recommended MVP defaults:

- global evaluation concurrency: configurable, default around `32`
- per-profile minimum evaluation interval: configurable, default around `5m`
- request path never starts unbounded evaluation work

Large candidate pools are evaluated over time. Evaluation ordering may use Node Observation data, recent usability, Egress Country, and broad latency estimates to choose what to test first, while keeping the remaining eligible candidates available for later background evaluation.

## Background Maintenance

Background Maintenance keeps Subscriptions, Node Observations, and Access Profiles fresh without requiring manual Web Console actions or proxy client requests.

The MVP uses separate Maintenance Schedules because each task has different cost and freshness needs:

- Subscription Refresh: default 6h
- Node Observation: default 30m
- Single-node Profile Evaluation: default 5m
- Chain Profile Evaluation: default 15m
- GeoIP Update: default daily at `07:00` local time

Each remote Subscription may override or disable its Subscription Refresh schedule. Each Access Profile may override or disable its Profile Evaluation schedule. Node Observation uses a global schedule in the MVP to keep the Web Console simple.

Maintenance Schedule configuration is split into two levels:

- System Settings: global defaults for Subscription Refresh, Node Observation, Single-node Profile Evaluation, Chain Profile Evaluation, and GeoIP Update
- Resource detail pages: per-Subscription overrides for Subscription Refresh and per-Access Profile overrides for Profile Evaluation

The MVP does not support per-Node observation schedules. Nodes can still be observed immediately through a manual action.

Successful Subscription Refresh triggers Node Observation for new or changed Nodes. Completed Node Observation may trigger Profile Evaluation for affected Access Profiles.

Background Maintenance also reacts to configuration changes:

- creating a remote Subscription triggers an immediate Subscription Refresh
- successful Subscription Refresh triggers Node Observation for added or changed Nodes
- successful Manual Node Import triggers Node Observation for that Node
- creating or updating an Access Profile triggers Profile Evaluation for that profile
- completed Node Observation triggers Profile Evaluation for affected Access Profiles

These follow-up actions run through Maintenance Runs. Saving a resource should not block on all follow-up work, but the Web Console must show related Maintenance Runs and their structured details.

Maintenance Runs use the following status model:

- state: `queued`, `running`, or `finished`
- result: empty until finished, then `success`, `warning`, `failure`, `skipped`, or `cancelled`
- reason code: empty until finished, then a stable localizable code explaining the result
- last error: a real execution exception summary only, not a user-facing sentence and not a normal skip or cancellation reason

`success` means the maintenance goal was achieved according to product rules. Normal ignored, skipped, unchanged, or deduplicated Subscription entries do not make a Subscription Refresh a warning. `warning` means some expected target failed while the overall maintenance still produced a usable result or preserved existing usable state. `failure` means the maintenance goal was not achieved or left the related resource without a usable result. `skipped` means the system decided the run or task should not execute or should not apply its result. `cancelled` means the run or task was explicitly terminated or replaced.

Maintenance Run cards use only common run fields: name, state, result, reason code, total count, finished count, and timestamps. Type-specific counts and summaries belong in detail data and are shown only after opening the run. `total_count` and `finished_count` are common coarse progress fields; success, warning, failure, skipped, cancelled, candidate, import, and failure-reason counts stay in type-specific detail data. The Home overview card title is `维护历史`.

Background Maintenance tasks must be deduplicated and bounded at the worker level:

- manual Node Observation for all Nodes replaces older unfinished all-Node observation runs
- scheduled Node Observation is skipped when a previous scheduled observation run is still unfinished
- manual Node Observation for a single Node may create another queued task for that Node
- if Profile Evaluation for the same Access Profile is already queued, additional triggers are merged
- if Profile Evaluation for the same Access Profile is running, additional triggers are merged into one pending rerun rather than starting a concurrent cycle
- when the running cycle finishes and a pending rerun exists, a new cycle starts from the latest Access Profile configuration and latest Node state
- if Profile Evaluation is running and the Access Profile changes, the running result must not overwrite the newer configuration; the profile is marked for another evaluation
- manual "run now" actions have higher priority than scheduled maintenance
- Subscription Refresh, Node Observation, and Profile Evaluation each have separate concurrency limits
- large chain candidate sets continue across background work instead of blocking proxy requests

The Home overview shows recent Maintenance Runs. Opening a Maintenance Run shows its structured detail data. Node Observation details summarize observed Nodes and representative failures. Subscription Refresh details keep Node-level import changes as summary data rather than one task per imported Node. Profile Evaluation details summarize candidates, failures, best observed path, selected path, and why the current Proxy Path was or was not switched, without storing every candidate path as a separate long-term record. Explicit Access Profile switch actions are recorded as `profile_switch` Maintenance Runs.

Background Maintenance failures should be visible without interrupting normal use:

- failed Node Observation records the latest error and failure time
- failed Profile Evaluation records the latest error and moves the Access Profile to the appropriate Profile State
- background failures are shown through status badges, task status, and detail panels rather than repeated pop-up notifications
- manual "run now" actions return their result directly to the admin who triggered them
- transient background failures must not erase the last known usable Proxy Path

When a Node Observation fails but the service has a previous egress IP or Egress Country, the Web Console may keep showing the last known values while marking them as stale. Access Profiles may continue using a last known Proxy Path while it is still considered usable; if fresh evaluation fails but a previous path remains available, the Profile State is `degraded`. If no usable Proxy Path exists, the Profile State is `failed` and proxy requests fail quickly.

## Profile State

Access Profiles expose a Profile State:

- `pending`: created but no completed evaluation yet
- `running`: evaluation is currently running
- `ready`: at least one usable Proxy Path is available
- `degraded`: last known path exists but recent evaluation quality has dropped
- `no_candidate`: Candidate Filter produced no Nodes
- `failed`: candidates exist but all evaluation attempts failed
- `invalid_config`: the Access Profile references a Node, Subscription, or other configuration object that no longer exists

Access Profiles do not have a separate enabled/disabled admin state in the MVP. Profile State describes serving readiness only. To stop an access entry, the admin disables or deletes the Proxy Credentials under that Access Profile.

Having zero enabled Proxy Credentials does not change an Access Profile's Profile State to `failed`. Credential login availability and Proxy Path serving readiness are displayed separately. Access Profile summaries expose both total Proxy Credential count and enabled Proxy Credential count.

Protocol configuration failures discovered during Subscription import are skipped before creating Nodes. Protocol configuration or runtime failures discovered during Node Observation or Profile Evaluation mark the Node or Proxy Path as unusable for that evaluation. They do not produce `no_candidate` unless Candidate Filter produced no Nodes at all.

For Fixed Node Profiles, Profile State follows the fixed Node's serving readiness: usable and enabled is `ready`; observation not yet completed is `pending`; Disabled, unusable, or otherwise unable to serve is `failed`. The selected fixed Node is still used for proxy requests whenever it is usable and enabled.

For Access Profiles with an Egress Country Candidate Filter, Nodes whose Egress Country is unknown are controlled by the explicit Unknown Egress Country option. The Web Console should surface how many otherwise eligible Nodes have unknown Egress Country and whether observation is pending or running.

Deleting a Subscription or Manual Node Import must not silently rewrite Access Profiles. If a Fixed Node Profile references a Node whose last Node Source was removed, or a Chain Access Profile's Exit Node Set references a Node whose last Node Source was removed, the Access Profile becomes `invalid_config`. If a Candidate Filter references a deleted Subscription, the Access Profile also becomes `invalid_config`. The Web Console must prompt the admin to choose a replacement Node or Subscription. Proxy Request Logs keep their historical selected path metadata even if the referenced Node is later removed.

If a Fixed Node Profile references a Disabled Node, or a Chain Access Profile's Exit Node Set contains Disabled Nodes, the configuration is still valid but cannot currently produce a usable path through those Nodes. The Access Profile becomes `failed` when no usable alternative is possible, or `degraded` if it can continue using another last known path that does not require the Disabled Node.

When no usable Proxy Path is available, proxy requests fail quickly:

- HTTP Proxy returns `502 Bad Gateway`
- SOCKS5 returns an appropriate failure response

Proxy requests do not wait for Profile Evaluation.

## Target Connection Selection

A Target Connection is one proxied connection to a destination.

Selection rules:

- HTTP CONNECT: select once per CONNECT tunnel.
- SOCKS5 CONNECT: select once per CONNECT command.
- HTTP proxy absolute-form request: select for the upstream target connection.
- Existing Target Connections never switch Proxy Path mid-connection.
- Random Profile randomizes per new Target Connection.

## Proxy Request Logs

The MVP records Proxy Request Logs as metadata only.

Fields:

- timestamp
- Access Profile id, name, and Profile Identifier
- Proxy Credential id and remark
- target host and port
- selected Proxy Path with node ids and display names
- success or failure
- failure stage and error summary
- duration
- ingress and egress byte counts
- HTTP status code when available

Proxy Request Logs store Access Profile and Proxy Credential display fields as snapshots. Hard-deleting an Access Profile or Proxy Credential must not make historical logs unreadable. When a Proxy Credential is hard-deleted, `proxy_credential.id` on historical log entries still returns the original credential id from the stored snapshot; `proxy_credential.id` is `null` only when the request could not be attributed to any credential (for example, the Profile Identifier did not exist or the password did not match any credential).

Logs filter dropdowns list only currently existing Access Profiles and Proxy Credentials. Deleted resources remain visible in historical log rows through their stored snapshots, but they are not offered as active dropdown options. If `GET /api/request-logs` receives a deleted `access_profile_id` or `credential_id`, the backend still filters against the snapshot ids stored on historical logs.

Deleting an Access Profile is a hard delete of that Access Profile, its current Proxy Credentials, and its Profile Evaluation / switch history events. The Web Console must show a confirmation before deletion, warning that current proxy URLs immediately stop working and historical logs remain.

Deleting one Proxy Credential is also a hard delete. The Web Console must show a confirmation before deletion, warning that the credential's proxy URL immediately stops working and historical logs remain.

Authentication failures are logged only when they can be attributed to a known disabled Proxy Credential. If the Profile Identifier does not exist, or the Profile Identifier exists but the password does not match any Proxy Credential under it, the service returns authentication failure without writing a Proxy Request Log. When a disabled Proxy Credential is used, the service still records a failed Proxy Request Log with `failure_stage` set to `authentication`, including the matched Access Profile and Proxy Credential snapshot. This makes revoked-client activity visible in the Web Console.

The Web Console reads Proxy Request Logs through server-side pagination and structured filters. `GET /api/request-logs` accepts `page`, `page_size`, `access_profile_id`, `credential_id`, `result`, `target`, and `node_id`, and returns the current page plus the total matching count. Credential filter options list current Proxy Credentials; historical logs for hard-deleted credentials remain searchable by Access Profile, target, result, and time.

Explicit path switches, including `switch-to-best-observed`, are not Proxy Request Logs because they are not proxied Target Connections. They should be recorded in Profile Evaluation or Background Maintenance history and surfaced on the Access Profile detail as the latest switch reason, for example "管理员手动切换到当前观测最快路径".

Logs remains scoped to Proxy Request Logs only. Maintenance history is separate from Proxy Request Logs. Access Profile detail should expose recent Profile Evaluation and switch events from Maintenance Runs related to that Access Profile.

Proxy Request Log retention is configurable through System Settings. By default, retention is enabled with a 30-day period. A Background Maintenance task periodically deletes logs older than the retention period. When retention is disabled, logs are kept indefinitely. The retention setting is independent of other Maintenance Schedules.

The MVP must not record:

- request body
- response body
- packet contents
- full request/response header capture by default

## Web Console

The web console should include these primary areas.

The web console must provide admin-console-grade mobile support, not a separate mobile-app experience. Core management flows must work on phones, including first-run setup, login, viewing Nodes and Access Profiles, creating Subscriptions, creating manual Nodes, creating Access Profiles, running observations and evaluations, and creating Proxy Credentials. Dense tables may collapse into card-style rows or controlled horizontal scrolling on mobile, but form controls and action buttons must remain reachable without overlapping or clipped text. A `375px` wide viewport is the minimum mobile acceptance target.

### Dashboard

- total Nodes
- usable Nodes
- Nodes by Egress Country
- Access Profile states
- recent failures
- recent Target Connection volume

### Subscriptions

- list Subscriptions
- add remote Subscription
- add local Subscription
- inspect a Subscription detail, including source URL or pasted content and the latest import summary
- edit Subscription name, source URL or pasted content, and refresh schedule policy
- enable/disable Subscription
- refresh Subscription
- show parse warnings and last refresh result

### Nodes

- list deduplicated Nodes
- inspect one Node detail
- show Node Sources
- show Node Observation
- filter by Egress Country, source, protocol, name, and user-facing Node state
- import manual Nodes
- disable a Node
- trigger immediate Node Observation for one Node or all Nodes without asking for a Test URL

The Nodes view must show the same core information on desktop and mobile. Desktop may use a table and mobile may use card rows, but neither layout should hide important status fields.

Core fields:

- name
- protocol
- source
- address
- Egress Country
- egress IP
- usability
- basic latency
- last observation time
- latest error

The Nodes API supports server-side filtering for the Nodes view. `GET /api/nodes` accepts `egress_country`, `source_type`, `source_id`, `protocol`, `state`, and `name`. `state` is a user-task state such as `enabled`, `disabled`, `usable`, `unusable`, or `pending_observation`, and the Web Console displays these values with Chinese labels.

`GET /api/nodes/:id` returns Node detail fields, including address, protocol, all Node Sources, full Node Observation, normalized outbound JSON or raw JSON, and related Access Profile summaries.

Node Observation actions use `POST /api/nodes/observations/run`; an empty request observes all Nodes, and `{ "node_id": "..." }` observes one Node.

Latest error should be shown as a short one-line summary by default with a way to inspect the full message, especially on mobile. Subscription import skipped-entry counts and parse errors belong in the Subscriptions view, not in the Nodes view.

Manual Node Import should be a low-friction import flow, not a full sing-box configuration editor. It supports:

- URI import for common proxy share links such as `ss://`, `vmess://`, `vless://`, `trojan://`, and `hysteria2://`
- sing-box outbound JSON import for advanced protocols and options
- a basic form for simple `http`, `socks`, and `direct` Nodes

Failed manual imports must show actionable parse errors. Successful imports are normalized into Nodes the same way Subscription entries are normalized.

### Access Profiles

- create and edit Access Profiles
- list Access Profile summaries
- inspect one Access Profile detail without loading all Access Profiles
- choose profile type
- configure Candidate Filter, including a clear Node Source selector instead of a bare "manual only" checkbox
- configure Test URL where Profile Evaluation needs target access latency, including Fastest and Chain Access Profiles
- choose fixed Node or Exit Node Set where relevant
- show current selected Proxy Path
- show Best Observed Proxy Path only inside evaluation details, not as the primary current path
- show Proxy Credential count
- show Path Evaluation Latency and evaluation target
- show Profile State and latest evaluation details
- show latest switch reason when the current served Proxy Path changed because of Switching Tolerance, failure recovery, or an explicit admin action
- show recent Profile Evaluation and switch events inside evaluation details, not on the Logs page

Chain Access Profiles must show the selected Front Node, selected Exit Node, the selected Exit Node's final Egress Country, Chain Evaluation Mode, and the relevant chain metric. `chain_link` mode shows Chain Link Evaluation results. `end_to_end` mode shows End-to-End Evaluation results for `Front Node -> Exit Node -> Test URL`. Candidate Filter summaries in chain mode must be visually distinguished from the fixed Exit Node set and labeled as applying to Front Nodes.

The primary current path card must show only the currently served Proxy Path. Evaluation details may show the Best Observed Proxy Path, its latency difference from the current path, the active Switching Tolerance values, and the reason it has not replaced the current path, such as not meeting the relative or absolute improvement threshold.

Access Profile detail includes `latest_switch_reason`, `latest_switch_at`, and `latest_switch_trigger`. `latest_switch_trigger` values are `tolerance`, `failure_recovery`, and `admin_manual`; the Web Console must display Chinese labels instead of raw enum values. Evaluation details should also include the latest 10 Profile Evaluation and switch events, without pagination.

Each recent Profile Evaluation or switch event includes:

- `id`
- `occurred_at`
- `type`: `evaluation_started`, `evaluation_completed`, `path_switched`, or `evaluation_failed`
- `trigger`: `schedule`, `manual`, `config_change`, `node_observation`, or `pending_rerun`
- `summary`
- `path_before`
- `path_after`
- `best_observed_path`
- `reason`

The Web Console should display event `summary` and necessary path information using Chinese labels. It must not directly show raw `type` or `trigger` enum values.

The event fields `path_before`, `path_after`, and `best_observed_path` all use `ProxyPathSummary`. The same summary shape is used for the current served path and Best Observed Proxy Path in Access Profile detail.

The manual evaluate action triggers Profile Evaluation but does not bypass Switching Tolerance. The Web Console should show an explicit "切换到当前观测最快路径" action in evaluation details only when the Best Observed Proxy Path is valid and different from the current served Proxy Path.

When editing a Chain Access Profile, if the admin changes the Exit Node Set to contain multiple Nodes while the Chain Evaluation Mode is `chain_link`, the Web Console must automatically switch the mode to `end_to_end` and show an inline hint or toast such as "多出口节点仅支持整链最快".

### Proxy Credentials

- create multiple credentials under an Access Profile
- edit credential remarks
- enable or disable credentials
- hard-delete credentials
- show copyable HTTP Proxy and SOCKS5 Proxy URLs
- show last used time

The backend returns complete `http_proxy_url` and `socks5_proxy_url` values for each Proxy Credential. The Web Console copies those returned values instead of deriving host, port, username, or password locally.

### Logs

- search Proxy Request Logs
- filter by Access Profile, credential, target, Node, country, result
- inspect failure summaries

## API Surface

The management API is authenticated with Admin Credentials.

MVP resource groups:

- `/api/overview`
- `/api/dictionaries/egress-countries`
- `/api/subscriptions`
- `/api/nodes`
- `/api/access-profiles`
- `POST /api/access-profiles/:id/actions/switch-to-best-observed`
- `/api/proxy-credentials`
- `/api/evaluations`
- `/api/request-logs`
- `/api/system`

API schemas may add fields over time, but documented Web Console contract fields must not be removed, renamed, or repurposed. API names should match the product language in `docs/glossary.md`.

`GET /api/system/settings` and `PATCH /api/system/settings` expose System Settings used by the Web Console, including `public_proxy_endpoint` for Proxy Access Address.

`POST /api/access-profiles/:id/actions/switch-to-best-observed` is the explicit force-switch action used by the Access Profile detail page. The backend must revalidate that the Best Observed Proxy Path is valid and different from the current served Proxy Path. On success, it switches the current served Proxy Path immediately, records a Profile Evaluation or Background Maintenance history event instead of a Proxy Request Log, stores the latest switch reason, and returns the updated Access Profile detail or a response shape sufficient for the detail page to refresh. If no valid different Best Observed Proxy Path exists, it returns `409 Conflict`.

## Persistence

The service stores configuration and runtime state in SQLite under `/data`.

Persisted data includes:

- Admin Credential hashes
- plaintext Proxy Credential passwords
- Subscriptions
- manual Nodes
- deduplicated Nodes
- Node Sources
- Node Observations
- Access Profiles
- Profile Evaluation results
- Proxy Request Logs
- System Settings, including Proxy Access Address

Docker deployment should require only:

- one published port
- one `/data` volume

## Deployment

Recommended Docker shape:

```yaml
services:
  proxy-gateway:
    image: proxy-gateway:latest
    ports:
      - "2260:2260"
    volumes:
      - "./data:/data"
    environment:
      - LISTEN_ADDR=:2260
      - DATA_DIR=/data
```

The service should be usable behind a reverse proxy, but it must also work by directly exposing its Single External Port.

## MVP Scope

MVP includes:

- Single External Port
- Web UI and management API
- Admin Credential and Proxy Credential separation
- HTTP Proxy
- SOCKS5 TCP CONNECT
- Subscription import
- manual Node add
- deduplicated Node pool
- Node Observation
- Access Profiles for original needs 3-9
- background Profile Evaluation
- Switching Tolerance
- SQLite persistence
- Proxy Request Logs without bodies
- Docker deployment

### Node Protocol Engine Migration Order

After the initial MVP slices, migrate Node dialing to the embedded sing-box Node Protocol Engine in this order:

- add a Node Protocol Engine interface while keeping existing behavior green
- route existing `direct`, `http`, and `socks` Nodes through the embedded sing-box dialer
- store and fingerprint normalized sing-box outbound JSON for Nodes
- support `shadowsocks` and `vmess` import with real sing-box dialing
- add the remaining sing-box remote proxy protocols
- expose Subscription skipped-entry summaries in the API and web console

### Usability and Reliability Implementation Priority

The usability and reliability improvements should be implemented in this order:

1. Web Console cleanup:
   - remove Test URL input from Node Observation flows
   - align desktop and mobile Nodes fields
   - replace the bare "manual only" checkbox with a Node Source selector
   - turn the old add-node panel into Manual Node Import
2. Current defect fixes:
   - support `https://` Test URLs for Profile Evaluation
   - show Subscription skipped-entry reasons in user-facing groups
   - keep Subscription refresh from accumulating stale Nodes
   - exclude Disabled Nodes from random and fastest-style selection
3. Background Maintenance:
   - run Subscription Refresh, Node Observation, Profile Evaluation, and GeoIP Update automatically
   - trigger follow-up maintenance immediately after relevant configuration changes
   - deduplicate, prioritize, and limit maintenance tasks
   - show task status and latest errors in the Web Console
4. Resin-compatible GeoIP and Egress IP Probe:
   - fetch `https://cloudflare.com/cdn-cgi/trace` through Nodes to discover egress IP
   - download `country.mmdb` from `MetaCubeX/meta-rules-dat`
   - resolve Egress Country locally from the GeoIP Database
5. Chain evaluation:
   - keep the Exit Node set explicit
   - apply Candidate Filter only to Front Node candidates
   - use Chain Link Evaluation for `chain_link` mode
   - use End-to-End Evaluation for `end_to_end` mode
   - for multi-exit `end_to_end`, evaluate `Front Node x Exit Node` complete Proxy Paths over time
   - use the first successful complete path immediately when no current path exists
   - apply Switching Tolerance before replacing a usable current path

MVP excludes:

- complete sing-box config editing
- SOCKS5 UDP/BIND
- arbitrary multi-hop chains
- multi-instance clustering
- full traffic capture
- complex Candidate Filter expression language

## Open Product Risks

- Subscription parser compatibility can grow quickly; unsupported fields must be visible as warnings rather than silent surprises.
- Chain evaluation can become expensive with large candidate pools; it must stay in Background Maintenance, expose progress, and avoid blocking proxy requests.
- Single-port protocol dispatch must carefully distinguish Web/API HTTP traffic from HTTP Proxy traffic.
- Egress Country depends on successful observation; profiles using country filters may be empty until observations are available.
- Some clients reuse proxy connections aggressively; selection semantics must be documented as Target Connection based, not page-view based.
