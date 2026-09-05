# 代码管理规范:成员直推 · 双端同步(2026-09-05 v2.0)

> 本文件是代码管理唯一权威,取代已删除的 PR 时代文档(COLLABORATION_PROPOSAL / COLLABORATION_CHARTER / UPSTREAM_COLLABORATION_STANDARD)。
> 定案:成员提交,不走 PR;双端(GitHub + Gitee)每次提交均同步;全程同一批 commit,SHA 永远一致。

## 一、仓库拓扑与身份

| 角色 | GitHub | Gitee |
|------|--------|-------|
| 上游主仓库 | xiaofang142/hivemtk | xhpmayun/hivemtk |
| 成员账号 | hivemtkbot(write 权限) | jungle-hero(成员,push 权限) |
| owner 账号 | xiaofang142(token 在凭据管理器) | xhpmayun(token 在凭据管理器) |
| fork 备份 | hivemtkbot/hivemtk | jungle-hero/hivemtk |

remote 布局:`upstream`(GitHub 上游)、`gitee-upstream`(Gitee 上游)、`github`/`gitee`(fork 备份)。

## 二、核心铁律

1. **双端同 SHA**:所有提交只在本地 master 上做,commit 后同一批 commit 直推双端。禁止在任一平台单独 squash/merge 产生平台特有提交(那是 PR 时代 sha 分叉的根源)。
2. **每轮提交必须双端同步**:改码 → 自测 → commit → `git push upstream master` + `git push gitee-upstream master` → 校验双端 SHA 一致,缺一不可。
3. **master 禁止 force-push**(初始历史对齐已完成一次,此后永不)。禁止删除已推送提交。
4. **commit 规范不变**:Conventional Commits 中文 subject(`type(scope): 描述`),`.githooks/commit-msg` 钩子继续启用;强制 LF;密钥/大文件禁止入库。
5. **直推前置自测**:windows + `GOOS=linux` 双平台编译、`go vet`、`go test ./... -count=1`,全绿才允许 push(涉及前端则 npm build,涉及推理栈则 smoke test)。
6. **分支策略**:日常直接 master;大改动可开 `feature/*` 本地分支,完成后在本地合并回 master 再双端直推(远端不留分支,推送即落 master)。

## 三、每轮提交标准闭环(SOP)

```bash
# 1. 同步(开工前)
git checkout master && git pull upstream master && git push gitee-upstream master   # 确保 Gitee 跟上
# 2. 开发 + 自测(五层架构铁律不变)
#    windows/linux 编译 + go vet + go test ./... -count=1
# 3. 提交
git add <files> && git commit -m "feat(user-server): xxx"
# 4. 双端同步(同一批 commit,同 SHA)
git push upstream master && git push gitee-upstream master
# 5. fork 备份 + 校验
git push github master && git push gitee master
git ls-remote upstream master; git ls-remote gitee-upstream master   # 两边 SHA 必须相同
```

## 四、历史沿革说明

- 2026-09-05 之前:PR/MR 模式(GitHub #11-#15,Gitee !1-!7),因双平台各自 squash 产生 sha 永久分叉,每次贡献需双份合并 + cherry-pick 重建。
- 2026-09-05 定案改版:成员直推 + 双端同 SHA。历史已在本次一次性对齐(双端 master = 同一批 commit),PR 时代文档已删除。
- 保留物:`.github/` 下 Issue/PR 模板保留,服务可能到来的外部贡献者;GIT_RULES.md / CONTRIBUTING.md 为上游原有治理文档,其中 PR 流程章节对外部贡献者仍适用,成员内部以本文件为准。
- 已合并的 PR/MR 记录(#12-#15 / !3-!7)是既成历史,不删除,仅此后不再产生新的。
