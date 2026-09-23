# CPA Manager Plus v1.13.1: upstream response-model provenance

## Conclusion and scope

Primary sources: [release API](https://api.github.com/repos/seakee/CPA-Manager-Plus/releases/tags/v1.13.1), [release](https://github.com/seakee/CPA-Manager-Plus/releases/tag/v1.13.1), and fetched tag **v1.13.1**, resolved to **e188cd8ac4a9ef7a9875d37480a301f29997905d**. Local comparison: plugin-store HEAD **8da220179bc334b9bb24a287bafc7a2bbef772ee**, sibling CPA HEAD **b773607e3e7756dc6020a291825e4eb08899595a**, plus explicitly identified in-progress plugin changes. Existing topic Markdown under docs informed this note's placement in the requested research subdirectory.

**The third-party manager does not discover the response model itself. It consumes an already-populated response_model field from CPA's usage queue, persists it separately, and displays it.** This is not request-log scraping and not the local native plugin's usage.handle SDK path. The traced collector/parser performs no response-body or SSE model extraction. A host that does not emit the field cannot gain this diagnostic merely by installing the manager. [Collector][collector] · [parser][parser]

## Exact collection → storage → UI path

1. **Transport:** auto first tries RESP subscription, then HTTP, then RESP queue polling; explicit modes are also supported. Subscription reads messages; HTTP/RESP polling passes popped payloads to the same processor. HTTP is authenticated **GET /v0/management/usage-queue?count=N**, with Authorization: Bearer management-key; its response is an array of JSON objects or JSON-encoded strings. This is a usage queue, not a logs endpoint. [Mode selection][modes] · [subscription][subscribe] · [HTTP client][http] · [collector][collector]
2. **Parsing/ingest:** processItems calls usage.NormalizeRaw, then InsertEvents. The normalizer reads three independent identities: requested = alias/requested_model/requestedModel; resolved = resolved_model/resolvedModel/model/model_name/modelName; response = **response_model/responseModel only**. It does not substitute resolved/model when response is absent. Its display Model prefers requested, then resolved—do not equate this manager's generic model field with a billing identity. [Collector][collector] · [parser][parser]
3. **Persistence:** Event.ResponseModel receives that parsed value; the migration adds nullable TEXT usage_events.response_model; inserts use nullString(ev.ResponseModel). Session, parent-session, access-token hash, generate and stream fields travel alongside it, but do not produce it. [Event assignment][event] · [migration][migration] · [insert][insert]
4. **Monitoring/UI:** the repository scans the column into item.ResponseModel, and the monitoring service copies it to a row with JSON key response_model,omitempty. The web adapter/row model carries it to row.responseModel. The UI shows requested first, routed when different from requested, and a response-model line/badge only when both response and resolved exist and differ. Missing response is not inferred; when resolved is absent, a supplied response appears in the tooltip rather than triggering mismatch. [Repository read][read] · [DTO][dto] · [service copy][service] · [web adapter][adapter] · [row model][rows] · [rendering][ui]

## Host dependency and limits

- The release's compatibility baseline is **CPA v7.3.2**, but its own upgrade notes explicitly say new metadata appears **only when CPA actually supplies the corresponding queue fields**. That baseline is not a guarantee of response-model support. CPA later added the official plugin-facing field in [`ac3849e5d981e85dd3f713aae0691d23d7b3a56c`](https://github.com/router-for-me/CLIProxyAPI/commit/ac3849e5d981e85dd3f713aae0691d23d7b3a56c). [Tagged release notes][notes]
- This tagged manager is a **consumer**, not evidence of which CPA executor first captures the upstream JSON/SSE model. The traced path does not require request logs or a manager-installed CPA patch. Conversely, it does require a capable producer. **The exact first supporting CPA release/commit, provider/stream coverage, and whether a particular deployment uses a modified host are not established by this manager tag.** Do not claim that all stock CPA versions support it, or that a custom fork is mandatory.
- A supplied upstream-reported model is metadata, not proof of the actual model weights executed. A name mismatch is not by itself proof of downgrade. **Requested alias, routed/billing model, and upstream-reported response model remain distinct.** The manager explicitly compares response against resolved rather than overwriting resolved. [Parser][parser] · [UI][ui]

## Local cpaplus comparison

| Layer | Proven local state |
|---|---|
| Host usage producer | The inspected historical host snapshot contains Model/Alias and response headers but no ResponseModel. CPA official commit [ac3849e5d981e85dd3f713aae0691d23d7b3a56c](https://github.com/router-for-me/CLIProxyAPI/commit/ac3849e5d981e85dd3f713aae0691d23d7b3a56c) adds UsageRecord.ResponseModel and propagates it through the usage adapter; the plugin consumes that raw usage field first and keeps observation as fallback. [Core record][core] · [official field][official] · [plugin DTO][sdk] · [RPC][rpc] |
| Host queue alternative | The sibling host's queue serializer also has model/alias but **no response_model**. Switching to the manager's queue transport would not recover a field the producer never serializes. [Queue DTO][queue] |
| Committed plugin baseline | The original baseline parsed only Model/Alias; subsequent commit `f4ed9eb` added independent response-model storage, UI, and observation fallback without changing billing. [Baseline handler][baseline-handler] · [Baseline ingest][baseline-ingest] |
| Current plugin consumer | `usage.handle` reads official `ResponseModel` first, accepts legacy responseModel/response_model compatibility fields, and only then relies on exact response observation. It persists source/conflict metadata separately from routed/resolved model. [Decoder](../../plugins/cpa-manager-plus/go/internal/ingest/ingest.go#L97-L135) · [API mapping](../../plugins/cpa-manager-plus/go/internal/store/analytics.go#L646-L664) · [UI helper](../../plugins/cpa-manager-plus/web/src/utils/eventStreamDisplay.js#L24-L48) |

**Current practical order:** when CPA includes the official `UsageRecord.ResponseModel` from commit [ac3849e5d981e85dd3f713aae0691d23d7b3a56c][official], the plugin consumes that usage value first. Older hosts, missing/empty official values, or unsupported deployments use the plugin's bounded exact-observation fallback; if that fallback cannot establish provenance, the value remains empty. Never derive it from routed Model, Alias, or an assumed response header.

**Do not add a second polling consumer casually:** the local HTTP usage-queue handler calls **PopOldest**; it is destructive, not an observational read. Additional consumers can compete for records, and ingesting queue events alongside SDK events would require deliberate correlation/deduplication to avoid duplicate accounting. [Queue handler][pop]

Static tagged-source and local-source inspection only; no live CPA deployment, request logs, or provider traffic was sampled. Consumer support does not establish producer fidelity. No production code or sibling-repository files were changed by this research; the existing dirty plugin worktree was left intact.

[collector]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/apps/manager-server/internal/collector/collector.go#L348-L435
[modes]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/apps/manager-server/internal/collector/collector.go#L140-L157
[subscribe]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/apps/manager-server/internal/collector/collector.go#L229-L274
[http]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/apps/manager-server/internal/httpqueue/client.go#L46-L129
[parser]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/apps/manager-server/internal/usage/event.go#L509-L564
[event]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/apps/manager-server/internal/usage/event.go#L590-L606
[migration]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/apps/manager-server/internal/repository/sqlite/migrate.go#L2906-L2921
[insert]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/apps/manager-server/internal/repository/usageevent/repository.go#L494-L507
[read]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/apps/manager-server/internal/repository/usagemonitoring/event_page_read.go#L258-L268
[dto]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/apps/manager-server/internal/service/monitoring/service.go#L831-L847
[service]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/apps/manager-server/internal/service/monitoring/service.go#L3555-L3569
[adapter]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/apps/web/src/features/monitoring/model/analyticsAdapters.ts#L1080-L1104
[rows]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/apps/web/src/features/monitoring/model/eventRows.ts#L108-L123
[ui]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/apps/web/src/features/monitoring/components/RealtimeEventsPanel.tsx#L1122-L1209
[notes]: https://github.com/seakee/CPA-Manager-Plus/blob/e188cd8ac4a9ef7a9875d37480a301f29997905d/docs/release-notes/v1.13.1-en.md#L36-L40
[core]: https://github.com/xinghaix/CLIProxyAPI/blob/b773607e3e7756dc6020a291825e4eb08899595a/sdk/cliproxy/usage/manager.go#L21-L62
[official]: https://github.com/router-for-me/CLIProxyAPI/commit/ac3849e5d981e85dd3f713aae0691d23d7b3a56c
[sdk]: https://github.com/xinghaix/CLIProxyAPI/blob/b773607e3e7756dc6020a291825e4eb08899595a/sdk/pluginapi/types.go#L1403-L1450
[rpc]: https://github.com/xinghaix/CLIProxyAPI/blob/b773607e3e7756dc6020a291825e4eb08899595a/internal/pluginhost/rpc_client.go#L610-L614
[queue]: https://github.com/xinghaix/CLIProxyAPI/blob/b773607e3e7756dc6020a291825e4eb08899595a/internal/redisqueue/plugin.go#L148-L183
[pop]: https://github.com/xinghaix/CLIProxyAPI/blob/b773607e3e7756dc6020a291825e4eb08899595a/internal/api/handlers/management/usage.go#L23-L42
[baseline-handler]: https://github.com/xinghaix/CLIProxyAPI-Plugins-Store/blob/8da220179bc334b9bb24a287bafc7a2bbef772ee/plugins/cpa-manager-plus/go/main.go#L167-L175
[baseline-ingest]: https://github.com/xinghaix/CLIProxyAPI-Plugins-Store/blob/8da220179bc334b9bb24a287bafc7a2bbef772ee/plugins/cpa-manager-plus/go/internal/ingest/ingest.go#L92-L128
