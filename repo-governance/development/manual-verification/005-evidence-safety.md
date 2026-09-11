# Evidence Safety and Accessibility

## Evidence Is an Artifact

A repository file or an immutable hosted result — not a claim in prose and not a message in a conversation.

Each record names:

| Field     | Holds                                         |
| --------- | --------------------------------------------- |
| commit    | the exact revision that was tested            |
| procedure | the command run, or the manual steps followed |
| time      | when it ran                                   |
| result    | the terminal outcome                          |
| artifact  | where the sanitized output is stored          |

"Tested and works" names none of these and can be neither audited nor reproduced.

## Text Alternatives Are Required

Every capture, diagram, and prototype carries a useful text alternative — one that says what the image shows in the
respect that matters, not what it is.

`Screenshot of the settings page` describes the file.
`Notification toggle appears below the fold at 375px, requiring a scroll the desktop layout does not` describes the
finding.

This is not only an accessibility obligation, though it is that. An image with no description is unreadable to every
automated check, every text search, and every future reader working from a terminal — which is most of them.

## Nothing Private Reaches Evidence

Evidence must contain no credential, token, personal data, absolute home path, internal hostname or address, topology
detail, or private repository identifier — and no raw scanner output that could reproduce any of them.

Captures are the usual leak. A screenshot taken while working carries whatever was on screen: a path, a branch name, a
session token in a URL, an adjacent window.

## Sanitize by Reconstructing, Not by Covering

Replace the value with a semantic placeholder — `<api-token>`, `<private-host>`, `<repository-path>` — or retake the
capture in a clean state.

Do not draw a box over it. Redaction applied on top of an image leaves the original data in the file more often than
anyone expects, and a box that can be removed is not a redaction.

## If It Cannot Be Sanitized, It Does Not Ship

An artifact whose meaning depends on the prohibited value stays private, and the evidence record describes it
structurally instead: what was checked, what the result was, and why the artifact itself cannot be published.

A finding summarized safely is worth more than a finding published unsafely, and there is no exception path for either.
