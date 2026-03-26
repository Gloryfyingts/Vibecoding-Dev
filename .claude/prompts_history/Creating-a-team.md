  Create a parallel Claude Code agent team called "vibecoders" using all agent definitions
  found in .claude/agents/ (planner, de-coder, local-repo-devops, reviewer).

  Set up the team with full task tracking enabled. Configure each agent's model and tool
  access exactly as specified in its agent file. Establish the following phase dependencies
  in the task list:

  1. planner runs first and blocks all other agents until its output (task_plan.md) is
     approved by the user
  2. multiple instances(if needed) of de-coder and local-repo-devops run in parallel after plan approval, scoped to their
     respective domains
  3. reviewer runs after all implementation agents finish, reviews against the definition
     of done, and gates completion

  Write a reusable orchestration claude-skill that I can reuse later with any task description to
  invoke this team in the correct sequence. The team should be ready to accept work
  immediately after setup.