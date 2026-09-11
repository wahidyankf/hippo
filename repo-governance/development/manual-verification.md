# Manual Verification

Automation proves that a stated property holds. It cannot prove that the property was the right one, that a correct
interface is usable, or that anything works on a device nobody ran it on.

This standard covers what is left, and it is deliberately narrow: manual verification is expensive, and a rule that
demands it everywhere gets satisfied ritually and stops meaning anything.

## Modules

1. [Quality Gate Results](manual-verification/001-quality-gate-results.md)
2. [Verification Layers](manual-verification/002-verification-layers.md)
3. [Exploratory and Usability Review](manual-verification/003-exploratory-and-usability.md)
4. [Interface Alternatives](manual-verification/004-interface-alternatives.md)
5. [Evidence Safety and Accessibility](manual-verification/005-evidence-safety.md)

## Applicability

Only layers relevant to the changed surface apply. A change with no user-facing surface records the interface layers as
not applicable, with a reason, and adds no screenshots to satisfy a checklist.

Recording "not applicable" is a real disposition and requires a real reason. Silence is not, because silence and
oversight look identical afterwards.
