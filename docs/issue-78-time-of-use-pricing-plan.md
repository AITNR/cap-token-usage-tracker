# Issue #78：模型峰谷定价

对应 https://github.com/AITNR/cap-token-usage-tracker/issues/78 。

## 实现范围

完整模式的模型价格编辑器新增峰谷定价折叠区。模型可配置 IANA 时区、时段名称、ISO 星期编号（1=星期一，7=星期日，留空为每天）、开始和结束时间，以及四项 USD/百万 token 单价。

- 每个模型最多 32 个时段；名称唯一，时段不允许重叠。
- 时间严格使用 HH:mm，起止相同被拒绝，不支持 24:00。
- 区间左闭右开；23:00–08:00 表示跨午夜，并归属起始日。例如星期日的该时段延续到星期一 08:00。
- 默认 UTC；禁止依赖机器的 Local 时区。内嵌 Go 时区数据，支持 Windows 和精简容器。
- 夏令时重复小时按当地钟表时间匹配，两次均适用；跳过的小时没有对应请求时刻。
- 按保存的请求时间选价，沿用上游缺少 RequestedAt 时的接收时间兜底，无法据此保证与供应商账单完全一致。

## 价格优先级

先匹配模型和 Service Tier，再应用时间价格，最后应用 Context Tier。时间价格覆盖四项单价；Context Tier 超阈值时覆盖时间价格。这一版不支持时间段内嵌上下文阶梯。

未命中使用基础/Service Tier 价格。请求费用提示和 CSV 增加计费时段、计费时区；当 Context Tier 实际覆盖时间价格时，不将时间段标为最终计费依据。

## 示例

以下仅为测试示例，不代表 DeepSeek 当前官方价格：

```json
{
  "prices": {
    "example-model": {
      "input": 4,
      "output": 8,
      "cache_read": 1,
      "cache_creation": 0,
      "time_zone": "Asia/Shanghai",
      "time_tiers": [
        {
          "name": "谷时",
          "start": "23:00",
          "end": "08:00",
          "input": 1,
          "output": 2,
          "cache_read": 0.25,
          "cache_creation": 0
        }
      ]
    }
  }
}
```

## 持久化、同步与兼容性

扩展现有价格账本 JSON，无请求记录迁移。没有时间字段的旧配置继续有效。保存增加价格账本 revision，历史请求查询与成本汇总按当前配置重新估算。

沿用手动价格整条保护：编辑时间计划后该模型成为手动价格，models.dev 同步不会更新其基础价格。对于仍标记 models.dev 的条目，同步更新基础价格时保留已有时间计划。没有新增同步计数字段，手动条目继续计入 SkippedManual。

旧程序不认识这些 JSON 字段；降级后重新保存价格可能丢失时间计划，降级前应备份数据库。

## 验证

Go 测试覆盖分钟边界、跨周午夜、星期限制、夏令时重复小时、非法配置、深拷贝、价格优先级、同步保留、历史查询缓存失效和重启持久化。浏览器回归覆盖时段输入、表单序列化回填、重叠拒绝和删除。

仓库没有 npm test；浏览器测试由 Go 调用 Playwright。Windows 上设置 CHROME_PATH 和 REQUIRE_BROWSER_TESTS=1 后执行 go test ./...。依赖使用 npm ci 安装，不下载浏览器。
