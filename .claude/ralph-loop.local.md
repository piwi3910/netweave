---
active: true
iteration: 3
max_iterations: 20
completion_promise: "COMPLETE"
started_at: "2026-01-09T14:21:02Z"
---

fix security gh issues.
Success criteria:
- Coverage >= 85%
- All tests green
- No lint errors
- Do not disable tests, always fix the issue

PROCESS:
1) Make the smallest change that moves toward success
2) Run tests or validation
3) Fix failures and repeat
4) If stuck after N iterations, summarise blockers and suggest next steps

Output:
<promise>COMPLETE</promise> when done.
