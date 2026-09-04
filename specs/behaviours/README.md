# Resource guard behaviours

These scenarios are shared by unit, local integration, and safe compiled-binary E2E adapters. The unit adapter executes every scenario and does not support an exemption tag or inventory. Integration and E2E exemptions are available only for a concrete boundary that cannot execute the scenario safely or faithfully. Every exemption must match the contract's exact approved inventory and name both its boundary and reason, so a new, removed, renamed, incomplete, or unknown exemption fails behavior compliance.

## Directory Map

- [`admission.feature`](admission.feature) specifies admission and resource classification.
- [`artifacts.feature`](artifacts.feature) specifies generated artifact and report privacy and retention.
- [`evidence.feature`](evidence.feature) specifies bounded runtime evidence and lifetime summaries.
- [`execution.feature`](execution.feature) specifies leases, generic concurrency mapping, and child-process stream behavior.
- [`public-cli.feature`](public-cli.feature) specifies the safe executable and Unix stream boundary.
- [`portability.feature`](portability.feature) specifies normalized macOS, Linux, cgroup, swap, and PSI evidence.
- [`quality-gates.feature`](quality-gates.feature) specifies strict module-local Go lint and Gherkin adapter enforcement.
- [`release.feature`](release.feature) specifies release-specific capacity evidence.
