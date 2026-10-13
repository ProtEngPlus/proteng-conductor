## What

<!-- what changed, one or two sentences -->

## Why

<!-- why this change is needed -->

## Type of change

- [ ] feat
- [ ] fix
- [ ] docs
- [ ] refactor
- [ ] test
- [ ] build/ci
- [ ] chore

## How to test

<!-- steps to verify locally -->

## Related

<!-- PRs in other repos that must ship with this one, as "Related: ProtEngPlus/<repo>#<number>", and the merge order.
     See "ของที่ต้องแก้คู่กันข้าม repo" in manual-guides-2023/CONTRIBUTING.md. Write "none" if this PR stands alone. -->

## Checklist

- [ ] Targets `dev` (not `main`), unless this is a `dev` → `main` release PR
- [ ] `.env.example` updated if new env vars were added
- [ ] No secrets / credentials committed
- [ ] `make check` passes locally (protengplus-frontend: `npm run format:check && npm run lint && npm run build`)
- [ ] Coupled changes in other repos are done and linked under Related
