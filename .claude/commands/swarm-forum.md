---
name: swarm-forum
description: 12-agent swarm planning forum — 3 teams of 4 planners debate internally, present cross-team, then all 12 converge on one final plan.
allowed-tools: Read, Grep, Glob, Agent, Write, TaskCreate, TaskUpdate, TaskGet, TaskList, SendMessage
---

# Swarm Forum

Use this skill for high-stakes planning where maximum adversarial coverage is needed. Three independent teams of 4 planners each develop competing plans, then all 12 agents debate cross-team to forge one final plan.

The vibecoders team already exists — do NOT create or delete it. Just spawn teammates into it.

## Core objective

Produce a final plan that is:
1. correct
2. complete
3. minimal in unnecessary work
4. explicit about tradeoffs and risks
5. ordered into a realistic execution sequence
6. grounded in the actual repository and task context
7. stress-tested from 12 independent perspectives across 3 competing teams

## Team structure

12 agents total, organized into 3 teams of 4. Each team has the same 4 personalities (Architect, Skeptic, Pragmatist, Verifier) — same as plan-forum — but operates independently.

| Team | Architect | Skeptic | Pragmatist | Verifier | Presenter |
|------|-----------|---------|------------|----------|-----------|
| Alpha | alpha-architect | alpha-skeptic | alpha-pragmatist | alpha-verifier | **Architect** |
| Bravo | bravo-architect | bravo-skeptic | bravo-pragmatist | bravo-verifier | **Skeptic** |
| Charlie | charlie-architect | charlie-skeptic | charlie-pragmatist | charlie-verifier | **Pragmatist** |

Each team assigns a **different personality as presenter** so that cross-team presentation carries diverse emphasis — Alpha's Architect presents structure, Bravo's Skeptic presents risks, Charlie's Pragmatist presents efficiency.

## Operating model

### Step 1: Create task list

Create these tasks using TaskCreate:

| ID | Task | Blocked By |
|----|------|------------|
| T1 | Phase 1 — Intra-team debate: each team of 4 produces a team plan | — |
| T2 | Phase 2 — Cross-team presentation: 3 presenters share plans with all 12 agents | T1 |
| T3 | Phase 3 — Full swarm debate: all 12 agents discuss and converge | T2 |
| T4 | Final synthesis — team lead merges into one plan | T3 |

### Step 2: Spawn all 12 planners as vibecoders teammates

All 12 agents are **teammates in the existing vibecoders team**. They communicate with each other via SendMessage during all 3 phases. They are NOT standalone sub-agents — they must be able to talk to each other.

Launch all 12 in a **single message** with 12 parallel Agent tool calls. All use `subagent_type: planner` and `team_name: vibecoders`.

Each agent gets a prompt with 3 sections:
1. **Identity** — team name, personality, teammates list, presenter role
2. **Personality prefix** — same as plan-forum
3. **3-phase workflow** — described below

#### Personality prefixes (reused across all 3 teams)

**Architect:**
> On top of your standard planning workflow, you place extra emphasis on designing the cleanest end-to-end approach, defining clear boundaries and interfaces between components, dependency ordering, and long-term maintainability. Optimize for coherence. When in doubt, prefer structure over shortcuts.

**Skeptic:**
> On top of your standard planning workflow, you place extra emphasis on attacking assumptions, finding hidden complexity, edge cases, missing constraints, and failure modes. Question whether each proposed step is actually necessary or overengineered. Be aggressive about finding hidden coupling, migration risk, test gaps, rollback blind spots, overconfidence, and dependency mistakes.

**Pragmatist:**
> On top of your standard planning workflow, you place extra emphasis on finding the fastest safe path, reducing scope, cutting ceremony, preferring leverage and iteration. Identify what can be deferred, mocked, staged, or simplified. Be aggressive about removing unnecessary scope, splitting into phases, reducing coordination cost, and producing value sooner.

**Verifier:**
> On top of your standard planning workflow, you place extra emphasis on checking repository facts and implementation reality. Validate that every proposed step matches the current codebase — actual files, actual conventions, actual integration points. Flag any speculation or unsupported claims. If something is assumed to exist, grep for it. If the repo contradicts the plan, the repo wins.

#### Prompt template for each agent

Use this template, filling in the placeholders per agent:

```
Agent:
  subagent_type: "planner"
  name: "{team}-{personality}"
  team_name: "vibecoders"
  model: "opus"
  prompt: |
    You are the {Personality} planner in TEAM {TEAM} of a 12-agent swarm forum.

    YOUR TEAM ({TEAM}): {team}-architect, {team}-skeptic, {team}-pragmatist, {team}-verifier
    OTHER TEAMS: {other teams' agent names}
    YOUR TEAM'S PRESENTER: {team}-{presenter-personality} (presents your team's plan to other teams)
    YOU ARE {THE PRESENTER / NOT the presenter}.

    YOUR PERSONALITY: {personality prefix from above}

    === PHASE 1 — INTRA-TEAM DEBATE ===
    Follow your standard planner workflow (read task, analyze codebase, identify scope,
    risks, execution order). Do NOT write to task_plan.md — keep your plan as text.
    When done, send your complete plan to your 3 TEAM teammates via SendMessage.
    Wait until you have received plans from all 3 teammates.
    Then critique, debate, and converge with your team — same as plan-forum Phase 2.
    Focus on YOUR specialty when critiquing.
    Continue debating until your team converges on ONE team plan.

    === PHASE 2 — CROSS-TEAM PRESENTATION ===
    {IF PRESENTER}: Take your team's converged plan and send it via SendMessage to
    ALL 8 agents on the other two teams. Label it clearly: "TEAM {TEAM} PLAN: ..."
    Wait until you receive the other 2 teams' plans from their presenters.
    Then forward the received plans to your own team via SendMessage.

    {IF NOT PRESENTER}: Wait until your team's presenter forwards you the other
    teams' plans. Read all 3 plans (yours + 2 received).

    === PHASE 3 — FULL SWARM DEBATE ===
    Now discuss with ALL 11 other agents across all teams via SendMessage.
    - Compare the 3 team plans
    - Identify where plans agree (high-confidence steps)
    - Identify where they disagree (needs resolution)
    - Argue for your team's approach where you believe it is stronger
    - Concede where another team's approach is genuinely better
    - Focus critiques on YOUR specialty
    - Work toward convergence on ONE final plan across all 12 agents
    - Do not allow shallow consensus — if you see a real problem, push back

    When the swarm reaches consensus, return your final position as output text.

    CRITICAL: Do NOT write to task_plan.md. Return your final plan as output text.

    Task: $ARGUMENTS
```

#### Full agent roster to spawn

**Team Alpha** (presenter: alpha-architect):

1. `alpha-architect` — Architect personality, IS the presenter
2. `alpha-skeptic` — Skeptic personality, NOT the presenter
3. `alpha-pragmatist` — Pragmatist personality, NOT the presenter
4. `alpha-verifier` — Verifier personality, NOT the presenter

**Team Bravo** (presenter: bravo-skeptic):

5. `bravo-architect` — Architect personality, NOT the presenter
6. `bravo-skeptic` — Skeptic personality, IS the presenter
7. `bravo-pragmatist` — Pragmatist personality, NOT the presenter
8. `bravo-verifier` — Verifier personality, NOT the presenter

**Team Charlie** (presenter: charlie-pragmatist):

9. `charlie-architect` — Architect personality, NOT the presenter
10. `charlie-skeptic` — Skeptic personality, NOT the presenter
11. `charlie-pragmatist` — Pragmatist personality, IS the presenter
12. `charlie-verifier` — Verifier personality, NOT the presenter

### Step 3: Wait and collect results

Wait for all 12 agents to complete. Each returns their final post-swarm-debate position as output text.

### Step 4: Synthesize final plan

As team lead, read all 12 final outputs and synthesize one plan.

Synthesis priorities:
- **Unanimous agreement across all 3 teams** → include without question
- **2 of 3 teams agree** → include, but check the dissenting team's reasoning
- **All 3 teams disagree** → evaluate on merits using the conflict hierarchy:
  - Verifiers win on repo facts
  - Skeptics win on risk identification
  - Pragmatists win on simplification (if correctness preserved)
  - Architects win on structure (if cost justified)
- **Unique insight from one team** → include if validated by repo evidence

Write the synthesized plan to `task_plan.md` in this format:

```markdown
## Task: [task name]

### Task Understanding
One concise paragraph on what is being solved.

### Chosen Approach
The selected strategy and why it won over alternatives.

### Rejected Alternatives
- [alternative] — [why rejected, which team proposed it]

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
- [only if truly unresolved after repo inspection and 12-agent debate]

### Swarm Notes
- Key disagreements resolved: [summary of what teams fought about and how it was settled]
- Confidence level: [high/medium/low based on degree of convergence]
```

### Step 5: Present to user

Show the final plan summary and ask for approval before any implementation begins.

## Quality bar

The final plan must:
- be better than any single team's plan alone
- show evidence of cross-team challenge and convergence
- remove redundant or decorative steps
- avoid vague phrases like "investigate further" unless unavoidable
- avoid pretending uncertainty is resolved when it is not
- prefer grounded assumptions over speculative design
- have a strict Definition of Done (inherited from planner agent rules)
- include Swarm Notes showing what was debated and how disagreements were resolved

## Rules

- All 12 planners are read-only — no code changes during planning
- Do not implement code changes unless the user explicitly asks for execution after planning
- The team lead must not skip the synthesis — never just pick one team's plan wholesale
- All 3 phases must complete before synthesis — do not short-circuit
- If any planner reports it cannot proceed (missing DDL, unclear requirements), escalate to user
- If the task is simple enough for 4 agents, suggest `/plan-forum` instead
- If the task is trivially simple, suggest a single planner instead

## Invocation examples

`/swarm-forum Design the complete data pipeline architecture from API ingestion through Flink processing to Iceberg storage with CDC, exactly-once semantics, and disaster recovery`

`/swarm-forum Plan a major refactoring of the ETL layer — 3 independent teams should stress-test the migration strategy before we commit`
