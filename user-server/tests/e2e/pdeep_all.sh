#!/usr/bin/env bash
# pdeep_all.sh - 运行全部平台端深度/链路测试（pdeep_*.sh + pflow_*.sh）
# 自动发现当前目录下所有 pdeep_*.sh 与 pflow_*.sh，逐个执行并汇总结果。
set +u
cd "$(dirname "$0")"
PASS_TOTAL=0; FAIL_TOTAL=0; RC=0
for f in $(ls pdeep_*.sh pflow_*.sh 2>/dev/null | grep -v -E 'pdeep_all.sh' | sort); do
  echo "############################################################"
  echo "# 运行 $f"
  echo "############################################################"
  if bash "$f"; then
    # 解析尾部结果
    line=$(grep -E "结果: PASS=" "$f.log" 2>/dev/null | tail -1)
    echo "[$f] 通过"
  else
    echo "[$f] 失败(退出码非0)"
    RC=1
  fi
done
echo "===== 平台端测试汇总 ====="
echo "（各脚本尾部已有 PASS/FAIL 统计）"
exit $RC
