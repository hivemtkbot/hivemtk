# 贡献者许可协议 (Contributor License Agreement)

本协议适用于所有向本项目（hivemtk 仓库）提交 Pull Request 或 Patch 的贡献者。

## 1. 定义

- **"项目"**：指 hivemtk 仓库（[Gitee](https://gitee.com/xhpmayun/hivemtk) / [GitHub](https://github.com/xiaofang142/hivemtk)）。
- **"贡献"**：指您向本项目提交的任何代码、文档、设计、翻译或其他内容（包括但不限于 Pull Request、Patch、Issue 文本）。
- **"您"**：指提交贡献的个人或实体。
- **"我们"**：指本项目的版权持有人（HiveMtk 团队）。

## 2. 您的承诺

通过向本项目提交贡献，您声明并承诺：

1. **原创性**：该贡献由您原创，或您有权以本协议条款提交。
2. **合法授权**：您已获得所有必要授权，可向本项目授予本协议规定的权利。
3. **无第三方权利侵犯**：该贡献不侵犯任何第三方的版权、专利、商标、商业秘密或其他权利。
4. **不包含机密信息**：该贡献不包含您对任何第三方负有保密义务的信息。
5. **理解 AGPL-3.0**：您已阅读并理解本项目使用 [AGPL-3.0](LICENSE) 开源协议，并同意您的贡献将按 AGPL-3.0 协议分发。

## 3. 授予的权利

您授予我们（项目版权持有人）以下全球性、永久、不可撤销、非独占、免版税的权利：

1. **使用**：使用、复制、修改、合并、发布、分发、再授权您的贡献。
2. **分发**：在 AGPL-3.0 协议下分发您的贡献。
3. **专利实施**：就您的贡献所包含的专利权，对我们及我们的被许可方实施该专利。

## 4. 您的保留权利

1. 您保留对您贡献的版权。
2. 您可继续在自己的项目中使用您的贡献。
3. 您可基于您的贡献创作衍生作品。

## 5. 不提供担保

您的贡献按"原样"提供，不附带任何明示或暗示的担保，包括但不限于适销性、特定用途适用性、非侵权性的担保。在任何情况下，您对因您的贡献引起或与之相关的任何索赔、损害或其他责任不承担责任。

## 6. 协议变更

我们保留修改本协议的权利，修改后的协议将在本文件更新后生效。继续向本项目提交贡献即视为接受修改后的协议。

## 7. 适用法律

本协议适用中华人民共和国法律（为本协议之目的，不考虑其法律冲突规则）。

---

## 如何签署

### 个人贡献者

向本项目提交第一个 Pull Request 时，请在 PR 描述中包含以下声明：

```
### Contributor License Agreement

I, [您的姓名] <[您的邮箱]>, certify that I have read and agree to the
Contributor License Agreement (CLA.md) of this project.

Signed-off-by: [您的姓名] <[您的邮箱]>
```

### 企业贡献者

企业雇员在提交贡献前，请确保：

1. 您的雇主已批准您为本项目做出贡献。
2. 您的雇主不要求对您的贡献拥有权利，或您的雇主已签署企业版 CLA（联系 `jideilvluoqun@gmail.com` 获取）。

## 通过 Git 签名（推荐）

使用 Git 自动签名：

```bash
git config --global user.name "Your Name"
git config --global user.email "your.email@example.com"
git commit -s -m "your commit message"
# -s 会自动追加 "Signed-off-by: Your Name <your.email@example.com>"
```

CI 会检查所有 commit 是否包含 `Signed-off-by` 行（参考 [Developer Certificate of Origin](https://developercertificate.org/)）。

---

**最后更新**: 2026-08-15
