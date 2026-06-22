\# Memory Specification Delta



\## ADDED Requirements



\### Requirement: Phase-1 memory candidates shall be limited to global preferences



The system SHALL only generate long-term memory candidates for `global` scope and `preference` memory type in Phase 1.



\#### Scenario: Generate a supported preference candidate



\- WHEN the user explicitly expresses a long-term preference

\- AND the preference can be mapped to a supported canonical key

\- THEN the system SHALL generate a `global + preference` candidate



\#### Scenario: Reject unsupported memory type



\- WHEN an extracted candidate has memory type other than `preference`

\- THEN the system SHALL reject the candidate before persistence



\#### Scenario: Reject unsupported scope



\- WHEN an extracted candidate has scope other than `global`

\- THEN the system SHALL reject the candidate before persistence



\#### Scenario: Do not generate knowledge candidates in Phase 1



\- WHEN the user expresses a possible long-term factual statement

\- THEN the system SHALL NOT automatically generate a `knowledge` memory candidate in Phase 1





\### Requirement: Preference candidates shall use an allowlisted canonical key



The system SHALL only allow Phase-1 preference candidates whose canonical key is in the supported allowlist.



The supported Phase-1 canonical keys are:



\- `response.language`

\- `workflow.troubleshooting.first\_step`

\- `behavior.avoid`



\#### Scenario: Accept allowlisted canonical key



\- WHEN a preference candidate uses an allowlisted canonical key

\- THEN the system SHALL allow it to continue through validation



\#### Scenario: Reject non-allowlisted canonical key



\- WHEN a preference candidate uses a canonical key outside the allowlist

\- THEN the system SHALL reject the candidate before persistence



\#### Scenario: Reject deprecated workflow key



\- WHEN a preference candidate uses `workflow.first\_step`

\- THEN the system SHALL reject the candidate

\- AND the system SHALL only accept the narrower `workflow.troubleshooting.first\_step` key for troubleshooting first-step preferences





\### Requirement: Preference extraction shall use a gated three-stage pipeline



The system SHALL extract preference candidates using a three-stage pipeline:



1\. Rule pre-filter

2\. LLM structured extraction

3\. Rule post-filter



\#### Scenario: Skip one-off input without preference trigger



\- WHEN the user input is a one-off algorithm question, transient error, temporary command, greeting, translation request, calculation request, or short contextual follow-up

\- AND the input does not contain a long-term preference trigger

\- THEN the system SHALL skip long-term memory extraction



\#### Scenario: Do not skip input with preference trigger



\- WHEN the user input contains a long-term preference trigger

\- THEN the system SHALL NOT skip the input only because it appears inside an algorithm, error, translation, or other one-off scenario

\- AND the system SHALL allow the input to enter structured extraction



\#### Scenario: Reject invalid LLM extraction result



\- WHEN the LLM extraction result contains an unsupported memory type, unsupported canonical key, low confidence, overly long content, sensitive content, or temporary wording

\- THEN the system SHALL reject the candidate before persistence





\### Requirement: Automatically generated candidates shall be pending by default



The system SHALL persist valid automatically generated preference candidates as `pending`.



\#### Scenario: Persist valid candidate as pending



\- WHEN a preference candidate passes pre-filter, LLM structured extraction, and post-filter

\- THEN the system SHALL persist the candidate with `pending` status



\#### Scenario: Pending candidate does not affect recall



\- GIVEN a preference candidate is in `pending` status

\- WHEN the system performs long-term memory recall

\- THEN the candidate SHALL NOT be included in recall results





\### Requirement: Users shall be able to confirm or reject pending preference candidates



The system SHALL provide backend API semantics for listing, confirming, and rejecting pending preference candidates.



\#### Scenario: List pending candidates



\- WHEN the user requests pending preference candidates

\- THEN the system SHALL return only candidates belonging to the current user

\- AND the system SHALL only include candidates in `pending` status



\#### Scenario: Confirm pending candidate



\- GIVEN a preference candidate is in `pending` status

\- WHEN the user confirms the candidate

\- THEN the system SHALL transition the candidate to `active`



\#### Scenario: Reject pending candidate



\- GIVEN a preference candidate is in `pending` status

\- WHEN the user rejects the candidate

\- THEN the system SHALL transition the candidate to `rejected`



\#### Scenario: Reject invalid state transition



\- GIVEN a preference candidate is not in `pending` status

\- WHEN the user attempts to confirm or reject the candidate

\- THEN the system SHALL reject the transition





\### Requirement: Active preferences shall participate in chat and agent recall



The system SHALL include only `active` preference memories in long-term memory recall for chat and agent flows.



\#### Scenario: Recall active preference in chat



\- GIVEN the user has an `active` preference memory

\- WHEN the user starts a related chat request

\- THEN the system SHALL make the active preference available to the chat prompt context



\#### Scenario: Recall active preference in agent flow



\- GIVEN the user has an `active` preference memory

\- WHEN an agent flow performs memory recall

\- THEN the system SHALL make the active preference available to the agent memory recall capability



\#### Scenario: Exclude pending and rejected preferences



\- GIVEN the user has `pending` or `rejected` preference candidates

\- WHEN the system performs long-term memory recall

\- THEN those candidates SHALL NOT be included in recall results





\### Requirement: Current-turn explicit instructions shall override historical preferences



The system SHALL treat the current user message as higher priority than recalled long-term preferences.



\#### Scenario: Current message overrides language preference



\- GIVEN the user has an active language preference

\- WHEN the current user message explicitly requests a different language

\- THEN the system SHALL follow the current message for that response



\#### Scenario: Current message overrides workflow preference



\- GIVEN the user has an active troubleshooting first-step preference

\- WHEN the current user message explicitly asks for a different workflow

\- THEN the system SHALL follow the current message for that response





\### Requirement: Memory extraction and recall failures shall fail open



The system SHALL NOT block the main chat response when preference extraction, persistence, or recall fails.



\#### Scenario: Extraction failure does not block response



\- WHEN preference extraction fails after the main response is generated

\- THEN the system SHALL keep the main response successful

\- AND the system SHALL record the failure using existing logs or metrics



\#### Scenario: Persistence failure does not block response



\- WHEN a valid candidate fails to persist

\- THEN the system SHALL keep the main response successful

\- AND the system SHALL record the failure using existing logs or metrics



\#### Scenario: Recall failure does not block response



\- WHEN long-term preference recall fails

\- THEN the system SHALL continue the chat or agent flow without recalled preferences

\- AND the system SHALL record the failure using existing logs or metrics

