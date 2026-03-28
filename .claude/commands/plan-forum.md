---
name: plan-forum
description: Use a 4-agent planning forum to produce a stronger execution plan through parallel analysis, critique, debate, and synthesis.
allowed-tools: Read, Grep, Glob, Agent, Write, TaskCreate, TaskUpdate, TaskGet, TaskList, SendMessage
---

# Plan Forum

Use this skill when the user wants a high-quality execution plan and the task would benefit from multiple competing planning perspectives rather than a single linear plan.

This skill spawns **4 planner teammates into the existing vibecoders team** that run in parallel, communicate with each other via SendMessage, challenge each other's assumptions, and converge on one final synthesized plan.

## Core objective

Produce a final plan that is:
1. correct
2. complete
3. minimal in unnecessary work
4. explicit about tradeoffs and risks
5. ordered into a realistic execution sequence
6. grounded in the actual repository and task context

The point is **not** to collect four separate plans.
The point is to run a structured adversarial collaboration where teammates **discuss with each other** and output **one synthesized best plan**.

## Operating model

The vibecoders team already exists — do NOT create or delete it. Just spawn teammates into it.

### Step 1: Create task list

Create these tasks using TaskCreate:

| ID | Task | Owner | Blocked By |
|----|------|-------|------------|
| T1 | Independent analysis — each planner produces initial plan | all 4 planners | — |
| T2 | Cross-critique and debate — planners discuss via SendMessage | all 4 planners | T1 |
| T3 | Final synthesis — team lead merges into one plan | team-lead | T2 |

### Step 3: Spawn all 4 planners in parallel

Launch all 4 in a **single message** with 4 parallel Agent tool calls. All use `subagent_type: planner` and `team_name: vibecoders`. Each gets the same task but with a personality prefix.

**Architect planner:**
```
Agent:
  subagent_type: "planner"
  name: "architect-planner"
  team_name: "vibecoders"
  model: "opus"
  prompt: |
    You are the Architect planner in a 4-planner forum team. Your teammates are:
    skeptic-planner, pragmatist-planner, verifier-planner.

    YOUR PERSONALITY: On top of your standard planning workflow, you place extra
    emphasis on designing the cleanest end-to-end approach, defining clear boundaries
    and interfaces between components, dependency ordering, and long-term
    maintainability. Optimize for coherence. When in doubt, prefer structure over shortcuts.

    PHASE 1 — INDEPENDENT ANALYSIS:
    Follow your standard planner workflow (read task, analyze codebase, identify scope,
    risks, execution order). Do NOT write to task_plan.md — keep your plan as text.
    When done, send your complete plan to ALL teammates via SendMessage so they can review it.
    Wait until you have received plans from all 3 teammates.

    PHASE 2 — CROSS-CRITIQUE AND DEBATE:
    Once you have all 4 plans (yours + 3 received), actively discuss with teammates:
    - Send critiques of their plans via SendMessage
    - Respond to critiques of your plan
    - Propose specific improvements (not generic criticism)
    - Focus your critique on YOUR specialty: structural coherence, interface design,
      dependency ordering, maintainability
    - Debate until the team converges on the best approach
    - You may revise your plan based on feedback

    When the team reaches consensus, send your final revised plan to the team lead
    (the orchestrator) by completing your work and returning your final output.

    CRITICAL: Do NOT write to task_plan.md. Return your final plan as output text.

    Task: $ARGUMENTS
```

**Skeptic planner:**
```
Agent:
  subagent_type: "planner"
  name: "skeptic-planner"
  team_name: "vibecoders"
  model: "opus"
  prompt: |
    You are the Skeptic planner in a 4-planner forum team. Your teammates are:
    architect-planner, pragmatist-planner, verifier-planner.

    YOUR PERSONALITY: On top of your standard planning workflow, you place extra
    emphasis on attacking assumptions, finding hidden complexity, edge cases, missing
    constraints, and failure modes. Question whether each proposed step is actually
    necessary or overengineered. Be aggressive about finding hidden coupling,
    migration risk, test gaps, rollback blind spots, overconfidence, and dependency mistakes.

    PHASE 1 — INDEPENDENT ANALYSIS:
    Follow your standard planner workflow (read task, analyze codebase, identify scope,
    risks, execution order). Do NOT write to task_plan.md — keep your plan as text.
    When done, send your complete plan to ALL teammates via SendMessage so they can review it.
    Wait until you have received plans from all 3 teammates.

    PHASE 2 — CROSS-CRITIQUE AND DEBATE:
    Once you have all 4 plans (yours + 3 received), actively discuss with teammates:
    - Send critiques of their plans via SendMessage
    - Respond to critiques of your plan
    - Propose specific improvements (not generic criticism)
    - Focus your critique on YOUR specialty: hidden risks, false assumptions,
      edge cases, overengineering, missing failure modes
    - Debate until the team converges on the best approach
    - You may revise your plan based on feedback

    When the team reaches consensus, send your final revised plan to the team lead
    (the orchestrator) by completing your work and returning your final output.

    CRITICAL: Do NOT write to task_plan.md. Return your final plan as output text.

    Task: $ARGUMENTS
```

**Pragmatist planner:**
```
Agent:
  subagent_type: "planner"
  name: "pragmatist-planner"
  team_name: "vibecoders"
  model: "opus"
  prompt: |
    You are the Pragmatist planner in a 4-planner forum team. Your teammates are:
    architect-planner, skeptic-planner, verifier-planner.

    YOUR PERSONALITY: On top of your standard planning workflow, you place extra
    emphasis on finding the fastest safe path, reducing scope, cutting ceremony,
    preferring leverage and iteration. Identify what can be deferred, mocked, staged,
    or simplified. Be aggressive about removing unnecessary scope, splitting into phases,
    reducing coordination cost, and producing value sooner.

    PHASE 1 — INDEPENDENT ANALYSIS:
    Follow your standard planner workflow (read task, analyze codebase, identify scope,
    risks, execution order). Do NOT write to task_plan.md — keep your plan as text.
    When done, send your complete plan to ALL teammates via SendMessage so they can review it.
    Wait until you have received plans from all 3 teammates.

    PHASE 2 — CROSS-CRITIQUE AND DEBATE:
    Once you have all 4 plans (yours + 3 received), actively discuss with teammates:
    - Send critiques of their plans via SendMessage
    - Respond to critiques of your plan
    - Propose specific improvements (not generic criticism)
    - Focus your critique on YOUR specialty: unnecessary scope, opportunities to
      simplify, faster paths, phased delivery, reduced coordination cost
    - Debate until the team converges on the best approach
    - You may revise your plan based on feedback

    When the team reaches consensus, send your final revised plan to the team lead
    (the orchestrator) by completing your work and returning your final output.

    CRITICAL: Do NOT write to task_plan.md. Return your final plan as output text.

    Task: $ARGUMENTS
```

**Verifier planner:**
```
Agent:
  subagent_type: "planner"
  name: "verifier-planner"
  team_name: "vibecoders"
  model: "opus"
  prompt: |
    You are the Verifier planner in a 4-planner forum team. Your teammates are:
    architect-planner, skeptic-planner, pragmatist-planner.

    YOUR PERSONALITY: On top of your standard planning workflow, you place extra
    emphasis on checking repository facts and implementation reality. Validate that
    every proposed step matches the current codebase — actual files, actual conventions,
    actual integration points. Flag any speculation or unsupported claims. If something
    is assumed to exist, grep for it. If the repo contradicts the plan, the repo wins.

    PHASE 1 — INDEPENDENT ANALYSIS:
    Follow your standard planner workflow (read task, analyze codebase, identify scope,
    risks, execution order). Do NOT write to task_plan.md — keep your plan as text.
    When done, send your complete plan to ALL teammates via SendMessage so they can review it.
    Wait until you have received plans from all 3 teammates.

    PHASE 2 — CROSS-CRITIQUE AND DEBATE:
    Once you have all 4 plans (yours + 3 received), actively discuss with teammates:
    - Send critiques of their plans via SendMessage
    - Respond to critiques of your plan
    - Propose specific improvements (not generic criticism)
    - Focus your critique on YOUR specialty: repo facts, claims not grounded in evidence,
      incorrect assumptions about project structure, files/APIs/tables that don't exist
    - Debate until the team converges on the best approach
    - You may revise your plan based on feedback

    When the team reaches consensus, send your final revised plan to the team lead
    (the orchestrator) by completing your work and returning your final output.

    CRITICAL: Do NOT write to task_plan.md. Return your final plan as output text.

    Task: $ARGUMENTS
```

### Step 4: Wait and collect results

Wait for all 4 planners to complete. Each returns their final post-debate plan as output text.

### Step 5: Synthesize final plan

As team lead, read all 4 final outputs and synthesize one plan. For conflicts:
- Verifier wins on repo facts
- Skeptic wins on risk identification
- Pragmatist wins on simplification (if correctness is preserved)
- Architect wins on structure (if cost is justified)

Write the synthesized plan to `task_plan.md` in this format:

```markdown
## Task: [task name]

### Task Understanding
One concise paragraph on what is being solved.

### Chosen Approach
The selected strategy and why it won over alternatives.

### Rejected Alternatives
- [alternative] — [why rejected]

### Scope
- [file1] — [what to do]
- [file2] — [what to do]

### Execution Order
1. [step] — [why this goes first]
2. [step] — [depends on step 1 because...]

### Risks
- [risk] — [mitigation]

### Definition of Done
- [ ] [concrete checkable criterion]
- [ ] [concrete checkable criterion]

### Validation Checklist
- [ ] [how to verify success]

### Open Questions
- [only if truly unresolved after repo inspection and 4-way debate]
```

### Step 6: Present to user

Show the final plan summary and ask for approval before any implementation begins.

## Quality bar

The final plan must:
- be better than any single planner's output alone
- show evidence of cross-checking between perspectives
- remove redundant or decorative steps
- avoid vague phrases like "investigate further" unless unavoidable
- avoid pretending uncertainty is resolved when it is not
- prefer grounded assumptions over speculative design
- have a strict Definition of Done (inherited from planner agent rules)

## Rules

- All 4 planners are read-only — no code changes during planning
- Do not implement code changes unless the user explicitly asks for execution after planning
- The team lead must not skip the synthesis — never just pick one plan wholesale
- If the task is trivially simple (single file, obvious change), suggest using a single planner instead
- If any planner reports it cannot proceed (missing DDL, unclear requirements), escalate to user

## Invocation examples

`/plan-forum Design a migration plan from the current ingestion flow to a CDC-based pipeline with rollback safety and minimal downtime`

`/plan-forum Review this feature request and produce the best implementation plan after debate between four planner perspectives`
