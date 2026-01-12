---
active: true
iteration: 1
max_iterations: 100
completion_promise: "COMPLETE"
started_at: "2026-01-12T13:47:32Z"
---

fix all linting issues detected and make sure all gh actions run successfully.
Success criteria:
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
