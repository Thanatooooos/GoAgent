# Daily Brief Specification Delta

## ADDED Requirements

### Requirement: Daily Brief shall provide a top-of-page trend insight module

The system SHALL provide a `Trend Insight` module at the top of the `Daily
Brief` page before the normal issue content.

#### Scenario: Render trend insight before brief content

- GIVEN a user opens the `Daily Brief` page
- AND a valid trend insight snapshot exists for the selected domain
- THEN the system SHALL render the trend insight before the normal brief
  headline and sections

#### Scenario: Do not block brief rendering when trend insight is absent

- GIVEN a user opens the `Daily Brief` page
- AND no valid trend insight snapshot exists
- THEN the system SHALL still render the normal brief page content
- AND it SHALL omit the trend module rather than fail the page

### Requirement: Trend insight shall describe domain change rather than only user preference drift

The system SHALL explain how the subscribed domain recently changed, not only
how the user's reading behavior changed.

#### Scenario: Explain field-level change

- GIVEN the system analyzes recent `AI` content
- WHEN open-weight model releases increase while agent tooling and cost
  competition also rise
- THEN the system SHALL describe those movements as recent field changes

#### Scenario: Do not treat click drift as sufficient field change evidence

- GIVEN the user clicked a different subset of articles
- WHEN the underlying domain evidence does not support a structural change
- THEN the system SHALL NOT emit a strong field-change claim based only on user
  behavior

### Requirement: Trend insight shall use recent comparison internally and background calibration secondarily

The system SHALL use a recent comparison window as the primary internal signal
source and a longer background window as secondary calibration.

#### Scenario: Detect recent rising signal

- GIVEN recent observations show a stronger share of one facet than the prior
  recent comparison window
- THEN the system SHALL allow that facet to become a rising signal candidate

#### Scenario: Calibrate recent signal against a longer background

- GIVEN a recent signal candidate exists
- WHEN the longer background comparison shows the signal is either reinforcing
  or contradicting a broader direction
- THEN the system SHALL use that background to soften, confirm, or contextualize
  the final judgment

### Requirement: User-facing copy shall avoid explicit window notation

The system SHALL avoid exposing explicit internal window notation such as `7d`
or `30d` in default user-facing trend copy.

#### Scenario: Render natural language instead of system notation

- GIVEN the trend insight is shown on the page
- THEN the system SHALL use natural phrasing such as "recently" or
  "over this stretch"
- AND it SHALL NOT require visible labels such as `7d vs prev 7d`

### Requirement: Trend insight shall be evidence-backed

The system SHALL back surfaced trend judgments with representative evidence
items.

#### Scenario: Attach evidence to surfaced signal

- GIVEN a trend signal is rendered
- THEN the system SHALL be able to show representative supporting items for
  that signal

#### Scenario: Suppress unsupported signal

- GIVEN a candidate signal lacks enough supporting evidence
- THEN the system SHALL suppress or downgrade that signal rather than present
  it as a strong judgment

### Requirement: Trend insight shall use reusable general analysis axes

The system SHALL classify observations using a reusable set of general analysis
axes across domains.

The minimum Phase-1 general axes are:

- `event_type`
- `value_chain_position`
- `openness`
- `actor_type`
- `change_nature`
- `region_scope`

#### Scenario: Label one observation with general axes

- GIVEN one normalized content item enters trend analysis
- THEN the system SHALL be able to assign zero or more supported general-axis
  values to that observation

#### Scenario: Allow unknown axis values

- GIVEN one observation cannot be confidently classified on one axis
- THEN the system SHALL allow that axis to remain unknown

### Requirement: AI trend insight shall use an AI-specific extension template in Phase 1

The system SHALL support a richer `AI` domain template in Phase 1.

The Phase-1 `AI` extension SHALL support at least:

- `capability_layer`
- `release_form`
- `competitive_theme`
- `adoption_direction`
- `constraint_source`

#### Scenario: AI item receives AI-specific extension labels

- GIVEN one `AI` observation is analyzed
- WHEN its content supports AI-specific interpretation
- THEN the system SHALL allow AI extension labels in addition to the general
  analysis axes

### Requirement: Trend insight shall expose a stable structured read model

The system SHALL expose one stable structured trend insight model to the
frontend rather than only one opaque paragraph.

#### Scenario: Return structured trend insight

- GIVEN the `Daily Brief` page read path returns a trend insight
- THEN the returned object SHALL include a headline, summary, surfaced signals,
  and supporting evidence groups

#### Scenario: Include optional background and methodology

- GIVEN the system has longer-window calibration or methodology detail
- THEN the trend insight model SHALL allow optional background and methodology
  fields

### Requirement: Trend insight shall fail open on insufficient data

The system SHALL fail open when the recent domain evidence is too weak for a
stable judgment.

#### Scenario: Omit module when samples are insufficient

- GIVEN the selected domain does not meet the minimum sample threshold
- THEN the system SHALL omit the trend module
- AND it SHALL continue rendering the normal `Daily Brief`

#### Scenario: Downgrade wording when confidence is weak

- GIVEN the system has some evidence but confidence is limited
- THEN the system SHALL prefer softer wording over strong directional claims
