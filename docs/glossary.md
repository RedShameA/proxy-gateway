# Proxy Gateway

A service that exposes a web console and proxy entrypoint for managing proxy nodes and selecting outbound paths.

## Language

**Single External Port**:
The deployment shape where the web console, management API, HTTP proxy, and SOCKS5 proxy are all reachable through one externally published port.
_Avoid_: single proxy port, mixed port

**Entrypoint**:
The public network listener that accepts web, API, HTTP proxy, and SOCKS5 proxy traffic before routing each connection to the matching internal handler.
_Avoid_: inbound, gateway, listener

**Proxy Access Address**:
The externally reachable host and optional port used when generating copyable HTTP Proxy and SOCKS5 Proxy URLs for Proxy Credentials. IPv6 addresses use bracket notation. The Chinese UI label is 代理访问地址.
_Avoid_: browser address, dashboard URL, web URL

**HTTP Proxy**:
The proxy protocol entry that accepts HTTP proxy requests and CONNECT tunnels through the Entrypoint.
_Avoid_: web proxy, HTTP inbound

**SOCKS5 Proxy**:
The proxy protocol entry that accepts authenticated SOCKS5 TCP CONNECT requests through the Entrypoint.
_Avoid_: SOCKS inbound, SOCKS tunnel

**Access Profile**:
A named proxy access configuration created in the web console. It owns one Profile Identifier and may have multiple Proxy Credentials for different clients or locations. The Chinese UI label is 访问策略.
_Avoid_: rule, config

**Profile Identifier**:
A unique admin-facing string on an Access Profile that appears as the proxy username in HTTP Proxy and SOCKS5 Proxy URLs and selects that Access Profile during proxy authentication. The Chinese UI label is 策略标识.
_Avoid_: profile username, connection identifier, slug

**Proxy Credential**:
A named proxy password under an Access Profile. Proxy clients authenticate with the Access Profile's Profile Identifier and one Proxy Credential password.
_Avoid_: account, user, token

**Proxy Credential Remark**:
The admin-facing note that distinguishes one Proxy Credential from another under the same Access Profile.
_Avoid_: display name, account name, username

**Admin Credential**:
The credential used to access the web console and management API.
_Avoid_: root password, dashboard password, management token

**Profile Evaluation**:
The background process that keeps an Access Profile's current usable node or proxy chain up to date.
_Avoid_: selection job, speed test, strategy calculation

**Profile Evaluation Cycle**:
One run of Profile Evaluation for an Access Profile, started from a snapshot of that Access Profile's current configuration.
_Avoid_: batch, round, scan

**Path Evaluation Latency**:
The latency measured by Profile Evaluation for a candidate Proxy Path.
_Avoid_: node latency, observation latency, ping

**Egress Country**:
The country or region of the IP address observed by the destination when traffic leaves the proxy path.
_Avoid_: node country, subscription country, label country

**Unknown Egress Country**:
The explicit state where Node Observation has not resolved a Node's Egress Country. It is selectable in country filters as 未知 instead of being silently included or excluded.
_Avoid_: empty country, no country

**Test URL**:
A URL used by Profile Evaluation to measure whether a proxy path can reach a target and how quickly the request completes.
_Avoid_: ping URL, latency URL, health URL

**Egress IP Probe**:
A lightweight endpoint used by Node Observation to reveal the IP address seen by the destination.
_Avoid_: IP test URL, country API, check IP site

**GeoIP Database**:
The local data source used to translate an observed egress IP address into an Egress Country.
_Avoid_: country service, location API, IP API

**Proxy Path**:
The node or two-hop chain used by a proxy request to reach its destination.
_Avoid_: route, line, tunnel

**Front Node**:
The first node in a two-hop Proxy Path.
_Avoid_: entry node, relay node, pre-node

**Exit Node**:
The final node in a Proxy Path; its observed IP determines the Egress Country.
_Avoid_: landing node, final node, outbound node

**Exit Node Set**:
The fixed one-or-more Exit Nodes selected on a Chain Access Profile. Candidate Filter does not apply to this set.
_Avoid_: exit filter, exit candidates

**Chain Link Evaluation**:
The Profile Evaluation mode that checks whether a Front Node can reach and complete the real proxy handshake with an Exit Node selected from the Exit Node Set.
_Avoid_: front speed test, port check, chain ping

**End-to-End Evaluation**:
The Profile Evaluation mode that measures a complete Proxy Path by fetching a Test URL through it.
_Avoid_: full speed test, target test, website test

**Chain Access Profile**:
An Access Profile whose Proxy Path is a two-hop chain from a Front Node to one selected Exit Node from a fixed Exit Node set. Its Chain Evaluation Mode decides how the Front Node is selected.
_Avoid_: fastest front profile, end-to-end chain profile

**Chain Evaluation Mode**:
The choice inside a Chain Access Profile between selecting the Front Node by Chain Link Evaluation or by End-to-End Evaluation. Chain Link Evaluation is valid only when the Exit Node Set has exactly one Exit Node.
_Avoid_: chain type, chain strategy type

**Switching Tolerance**:
The configurable rule that decides when a new or best-observed Proxy Path replaces the current usable Proxy Path, including relative improvement, absolute latency improvement, and maximum hold duration.
_Avoid_: evaluation budget, candidate cap, pair cap

**Best Observed Proxy Path**:
The fastest currently usable and still-valid Proxy Path most recently observed by Profile Evaluation for an Access Profile, tracked separately from the currently served Proxy Path.
_Avoid_: fastest variable, candidate winner

**Background Maintenance**:
Service-owned background work that keeps Subscriptions, Node Observations, and Access Profiles fresh without requiring a proxy client request.
_Avoid_: cron, scheduled jobs, auto tasks

**Maintenance Schedule**:
The configured cadence for a specific kind of Background Maintenance.
_Avoid_: refresh interval, timer, polling setting

**Maintenance Run**:
A single triggered instance of Background Maintenance, visible to the admin as one history record even when it contains many concrete work items.
_Avoid_: batch, round, scan, job log

**Maintenance State**:
The lifecycle phase of a Maintenance Run, separate from whether the work succeeded, failed, was skipped, or was cancelled.
_Avoid_: result, health, status

**Maintenance Result**:
The admin-facing result grade of a finished Maintenance Run, separate from its lifecycle state and from the detailed reason behind that result.
_Avoid_: state, error, status

**Maintenance Reason**:
A stable, localizable reason behind a Maintenance Result.
_Avoid_: error message, Chinese summary, raw exception

**Profile State**:
The current readiness of an Access Profile for serving Target Connections.
_Avoid_: status, health, phase

**Proxy Request Log**:
Metadata recorded about a Target Connection, excluding request and response bodies.
_Avoid_: traffic capture, access log, packet log

**Target Connection**:
A single proxied connection from a client request to one destination through a selected Proxy Path.
_Avoid_: request, session, tunnel

**Node**:
A proxy server configuration that can be used as part of a Proxy Path.
_Avoid_: server, endpoint, line

**Disabled Node**:
A Node that remains in the system but is intentionally excluded from proxy path selection.
_Avoid_: deleted node, unavailable node

**Node Observation**:
Runtime information measured about a Node, such as usability, observed exit IP, Egress Country, and latency.
_Avoid_: node status, health data, metrics

**Observation Latency**:
A basic latency sample captured during Node Observation by using a Node to reach the Egress IP Probe, independent of any Access Profile's target-specific selection. The Chinese UI label is 探测耗时.
_Avoid_: node ping, latency to node, profile latency, path latency, speed-test result

**Node Source**:
The source record that explains where a Node came from: a Subscription or a Manual Node Import. One deduplicated Node may have multiple Node Sources.
_Avoid_: provider, owner

**Subscription**:
A remote or local collection of Node definitions imported into the service.
_Avoid_: provider, feed, node list

**Subscription Import Summary**:
The result shown after importing or refreshing a Subscription, including how many Nodes were added, retained, updated, or removed and how many input entries were ignored or skipped.
_Avoid_: parser log, raw import output

**Ignored Subscription Entry**:
A Subscription input entry that is intentionally not a Node definition, such as an upstream strategy group or routing construct.
_Avoid_: skipped node, failed node

**Skipped Subscription Entry**:
A Subscription input entry that appears intended to define a Node but was not imported because it was unsupported, incomplete, malformed, or not dialable.
_Avoid_: skipped node, hidden node

**Manual Node Import**:
The flow that creates a Node directly from user-provided proxy configuration instead of from a Subscription.
_Avoid_: add node panel, manual protocol form

**Candidate Filter**:
The criteria an Access Profile uses to decide which Nodes can be considered for selection. In a Chain Access Profile, it applies only to Front Node candidates, not to the fixed Exit Node set.
_Avoid_: rule, matcher, selector
