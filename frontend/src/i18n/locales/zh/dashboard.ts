export default {
  dashboard: {
    title: '仪表盘',
    welcomeMessage: '欢迎回来！这是您账户的概览。',
    balance: '余额',
    apiKeys: 'API 密钥',
    todayRequests: '今日请求',
    todayCost: '今日消费',
    todayTokens: '今日 Token',
    totalTokens: '累计 Token',
    cacheToday: '今日缓存',
    performance: '性能指标',
    avgResponse: '平均响应',
    averageTime: '平均时间',
    timeRange: '时间范围',
    granularity: '粒度',
    day: '按天',
    hour: '按小时',
    modelDistribution: '模型分布',
    groupDistribution: '分组使用分布',
    platformBreakdown: '按平台拆分',
    platformBreakdownEmpty: '暂无平台用量',
    platformCount: '{count} 个平台',
    platformOther: '其他',
    platformQuota: {
      title: '配额用量',
      daily: '日',
      weekly: '周',
      monthly: '月（近30天）',
      resetsAt: '{time} 重置',
      noLimit: '不限制',
      disabled: '已禁用',
    },
    tokenUsageTrend: 'Token 使用趋势',
    noDataAvailable: '暂无数据',
    model: '模型',
    group: '分组',
    noGroup: '无分组',
    requests: '请求',
    tokens: 'Token',
    actual: '实际',
    standard: '标准',
    input: '输入',
    output: '输出',
    cache: '缓存',
    recentUsage: '最近使用',
    last7Days: '近 7 天',
    noUsageRecords: '暂无使用记录',
    startUsingApi: '开始使用 API 后，您的使用历史将显示在这里。',
    viewAllUsage: '查看全部',
    quickActions: '快捷操作',
    createApiKey: '创建 API 密钥',
    generateNewKey: '生成新的 API 密钥',
    batchImageAgent: '批量生图助手',
    batchImageAgentDesc: '复制给 Agent 的任务说明',
    viewUsage: '查看使用记录',
    checkDetailedLogs: '查看详细的使用日志',
    redeemCode: '兑换码',
    addBalanceWithCode: '使用兑换码充值'
  },

  // Groups (shared)
  groups: {
    subscription: '订阅'
  },

  // API Keys
  keys: {
    title: 'API 密钥',
    description: '管理您的 API 密钥和访问令牌',
    searchPlaceholder: '搜索名称或Key...',
    endpoints: {
      title: 'API 端点',
      default: '默认',
      copied: '已复制',
      copiedHint: '已复制到剪贴板',
      clickToCopy: '点击可复制此端点',
      speedTest: '测速',
    },
    allGroups: '全部分组',
    allStatus: '全部状态',
    columnSettings: '列设置',
    columnAlwaysVisible: '该列固定显示，不可隐藏',
    createKey: '创建密钥',
    editKey: '编辑密钥',
    deleteKey: '删除密钥',
    deleteConfirmMessage: "确定要删除 '{name}' 吗？此操作无法撤销。",
    id: 'ID',
    apiKey: 'API 密钥',
    group: '分组',
    currentConcurrency: '当前并发',
    noGroup: '无分组',
    searchGroup: '搜索分组...',
    noGroupFound: '未找到匹配的分组',
    created: '创建时间',
    copyToClipboard: '复制到剪贴板',
    copied: '已复制！',
    importToCcSwitch: '导入到 CCS',
    enable: '启用',
    disable: '禁用',
    nameLabel: '名称',
    namePlaceholder: '我的 API 密钥',
    groupLabel: '分组',
    selectGroup: '选择分组',
    statusLabel: '状态',
    selectStatus: '选择状态',
    saving: '保存中...',
    noKeysYet: '暂无 API 密钥',
    createFirstKey: '创建您的第一个 API 密钥以开始使用 API。',
    keyCreatedSuccess: 'API 密钥创建成功',
    keyUpdatedSuccess: 'API 密钥更新成功',
    keyDeletedSuccess: 'API 密钥删除成功',
    keyEnabledSuccess: 'API 密钥已启用',
    keyDisabledSuccess: 'API 密钥已禁用',
    failedToLoad: '加载 API 密钥失败',
    failedToSave: '保存 API 密钥失败',
    failedToDelete: '删除 API 密钥失败',
    failedToUpdateStatus: '更新 API 密钥状态失败',
    clickToChangeGroup: '点击更换分组',
    groupChangedSuccess: '分组更换成功',
    failedToChangeGroup: '更换分组失败',
    groupRequired: '请选择分组',
    usage: '用量',
    today: '今日',
    total: '近30天',
    quota: '额度',
    lastUsedAt: '上次使用时间',
    lastUsedIP: '最近使用 IP',
    useKey: '使用密钥',
    useKeyModal: {
      title: '使用 API 密钥',
      description: '将以下环境变量添加到您的终端配置文件或直接在终端中运行。',
      copy: '复制',
      copied: '已复制',
      note: '这些环境变量将在当前终端会话中生效。如需永久配置，请将其添加到 ~/.bashrc、~/.zshrc 或相应的配置文件中。',
      claudeSettingsHint: '用户级持久配置。此文件包含 API 密钥，请勿提交到项目仓库。',
      noGroupTitle: '请先分配分组',
      noGroupDescription:
        '此 API 密钥尚未分配分组，请先在密钥列表中点击分组列进行分配，然后才能查看使用配置。',
      openai: {
        description: '将以下配置文件添加到 Codex CLI 配置目录中。',
        authModeTitle: 'Codex 认证模式',
        authModeDescription: '兼容模式保留旧版 Codex 配置；API Key Mode 用于授权客户端图片执行器。',
        authModeLegacy: '兼容模式',
        authModeApiKey: 'API Key Mode',
        authModeApiKeyRestartNotice: '保存此配置后，必须完全退出并重启 Codex Desktop 或 CLI，然后新建 task，让客户端重新构建工具注册表。',
        configTomlHint: '请确保以下内容位于 config.toml 文件的开头部分',
        note: '请确保配置目录存在。macOS/Linux 用户可运行 mkdir -p ~/.codex 创建目录。',
        noteWindows:
          '按 Win+R，输入 %userprofile%\\.codex 打开配置目录。如目录不存在，请先手动创建。'
      },
      cliTabs: {
        claudeCode: 'Claude Code',
        geminiCli: 'Gemini CLI',
        codexCli: 'Codex CLI',
        codexCliWs: 'Codex CLI (WebSocket)',
        grokCli: 'Grok CLI',
        opencode: 'OpenCode'
      },
      antigravity: {
        description: '为 Antigravity 分组配置 API 访问。请根据您使用的客户端选择对应的配置方式。',
        claudeCode: 'Claude Code',
        geminiCli: 'Gemini CLI',
        claudeNote:
          '这些环境变量将在当前终端会话中生效。如需永久配置，请将其添加到 ~/.bashrc、~/.zshrc 或相应的配置文件中。',
        geminiNote:
          '这些环境变量将在当前终端会话中生效。如需永久配置，请将其添加到 ~/.bashrc、~/.zshrc 或相应的配置文件中。'
      },
      gemini: {
        description:
          '将以下环境变量添加到您的终端配置文件或直接在终端中运行，以配置 Gemini CLI 访问。',
        modelComment: '如果你有 Gemini 3 权限可以填：gemini-3-pro-preview',
        note: '这些环境变量将在当前终端会话中生效。如需永久配置，请将其添加到 ~/.bashrc、~/.zshrc 或相应的配置文件中。'
      },
      grok: {
        description:
          '配置 Grok CLI、Claude Code、Codex 或 OpenCode，让请求通过当前 spool Grok 分组发送。文本模型走 Responses；图片/视频使用 Imagine 模型 ID 与媒体端点。',
        claudeDescription: '配置 Claude Code，让 Messages API 请求通过当前 spool Grok 分组发送。',
        codexDescription: '配置 Codex，让 Responses API 请求通过当前 spool Grok 分组发送。',
        configTomlHint:
          '官方路径：~/.grok/config.toml（或 $GROK_HOME）。请填写 [endpoints]（models_base_url / models_list_url / xai_api_base_url / cli_chat_proxy_base_url）、[auth] preferred_method=api_key、[models]、[session]、[features] 图片/视频覆盖。优先 env_key，勿硬编码 api_key；文本模型必须 api_backend=responses。合并前备份，保存后运行 grok inspect。',
        codexConfigTomlHint:
          'Codex 官方：wire_api 仅支持 "responses"；优先 env_key，勿与 experimental_bearer_token 混用；非 OpenAI 网关默认 supports_websockets = false（spool 仍可接客户端 WS 并桥接到 HTTP/SSE）。合并前备份 ~/.codex/config.toml。',
        note:
          '导出 GROK_MODELS_BASE_URL 与 XAI_API_KEY，将完整 config.toml（endpoints/auth/models/session/features）保存为 ~/.grok/config.toml，运行 grok inspect，再用 /model 选择 grok-4.5（编程场景可用 grok-build-0.1）。',
        noteWindows:
          '设置 GROK_MODELS_BASE_URL 与 XAI_API_KEY，将完整 config.toml 保存为 %USERPROFILE%\\.grok\\config.toml，运行 grok inspect，再用 /model 选择 grok-4.5（编程场景可用 grok-build-0.1）。',
        claudeNote:
          '二选一：终端环境变量仅当前会话；~/.claude/settings.json 可持久化。请勿把含 API Key 的文件提交到仓库。',
        codexNote:
          '导出 SUB2API_API_KEY，将 config.toml 保存到 ~/.codex（可用 mkdir -p ~/.codex）。优先 env_key，勿提交密钥。',
        codexNoteWindows:
          '设置 $env:SUB2API_API_KEY，将 config.toml 保存到 %USERPROFILE%\\.codex。优先 env_key，勿提交密钥。'
      },
      deepseek: {
        description: '通过当前 DeepSeek 分组配置 Claude Code、Codex 或 OpenCode。',
        codexDescription: '使用 API Key 配置 Codex，并通过当前 DeepSeek 分组发送请求。',
        codexConfigTomlHint: '下载下方模型目录，将两个文件保存到 Codex 配置目录后重启 Codex。',
        codexNote: '启动 Codex 前先导出 SUB2API_API_KEY。下载的目录只包含模型元数据，不包含 API Key。'
      },
      minimax: {
        description: '通过当前 MiniMax 分组配置 Claude Code、Codex 或 OpenCode。',
        codexDescription: '使用 API Key 配置 Codex，并通过当前 MiniMax 分组发送请求。',
        codexConfigTomlHint: '下载下方模型目录，将两个文件保存到 Codex 配置目录后重启 Codex。',
        codexNote: '启动 Codex 前先导出 SUB2API_API_KEY。下载的目录只包含模型元数据，不包含 API Key。'
      },
      composite: {
        description: '通过当前 Composite 路由分组配置受支持的客户端。',
        codexDescription: '使用 API Key 和当前 Composite 分组的完整模型目录配置 Codex。',
        codexConfigTomlHint: '下载下方模型目录，将两个文件保存到 Codex 配置目录后重启 Codex。',
        codexNote: '启动 Codex 前先导出 SUB2API_API_KEY；分组会根据目录中选中的模型路由请求。'
      },
      routedCodex: {
        description: '使用当前路由分组的完整模型目录配置 Codex。',
        configTomlHint: '下载下方模型目录，将两个文件保存到 Codex 配置目录后重启 Codex。',
        note: '启动 Codex 前先导出 SUB2API_API_KEY。下载的目录只包含模型元数据，不包含 API Key。'
      },
      codexModelCatalog: {
        title: 'Codex 模型目录',
        description: '使用当前 API Key 获取目录，并保存到 config.toml 引用的路径。',
        fetch: '获取目录',
        retry: '重试',
        download: '下载目录',
        modelsCount: '已获取 {count} 个模型',
        errorDescription: '无法使用当前 API Key 获取模型目录。'
      },
      opencode: {
        title: 'OpenCode 配置示例',
        subtitle: 'opencode.json',
        hint: '配置文件路径：~/.config/opencode/opencode.json（或 opencode.jsonc），不存在需手动创建。可使用默认 provider（openai/anthropic/google）或自定义 provider_id。API Key 支持直接配置或通过客户端 /connect 命令配置。示例仅供参考，模型与选项可按需调整。'
      }
    },
    customKeyLabel: '自定义密钥',
    customKeyPlaceholder: '输入自定义密钥（至少16个字符）',
    customKeyHint: '仅允许字母、数字、下划线和连字符，最少16个字符。',
    customKeyTooShort: '自定义密钥至少需要16个字符',
    customKeyInvalidChars: '自定义密钥只能包含字母、数字、下划线和连字符',
    customKeyRequired: '请输入自定义密钥',
    ipRestriction: 'IP 限制',
    ipWhitelist: 'IP 白名单',
    ipWhitelistPlaceholder: '192.168.1.100\n10.0.0.0/8',
    ipWhitelistHint: '每行一个 IP 或 CIDR，设置后仅允许这些 IP 使用此密钥',
    ipBlacklist: 'IP 黑名单',
    ipBlacklistPlaceholder: '1.2.3.4\n5.6.0.0/16',
    ipBlacklistHint: '每行一个 IP 或 CIDR，这些 IP 将被禁止使用此密钥',
    ipRestrictionEnabled: '已配置 IP 限制',
    ccSwitchNotInstalled:
      'CC-Switch 未安装或协议处理程序未注册。请先安装 CC-Switch 或手动复制 API 密钥。',
    ccsClientSelect: {
      title: '选择客户端',
      description: '请选择您要导入到 CC-Switch 的客户端类型：',
      claudeCode: 'Claude Code',
      claudeCodeDesc: '导入为 Claude Code 配置',
      geminiCli: 'Gemini CLI',
      geminiCliDesc: '导入为 Gemini CLI 配置'
    },
    // 配额和有效期
    quotaLimit: '额度限制',
    quotaAmount: '额度金额 (USD)',
    quotaAmountPlaceholder: '输入 USD 额度限制',
    quotaAmountHint: '设置此密钥可消费的最大金额。0 = 无限制。',
    quotaUsed: '已用额度',
    reset: '重置',
    resetQuotaUsed: '将已用额度重置为 0',
    resetQuotaTitle: '确认重置额度',
    resetQuotaConfirmMessage: '确定要将密钥 "{name}" 的已用额度（${used}）重置为 0 吗？此操作不可撤销。',
    quotaResetSuccess: '额度重置成功',
    failedToResetQuota: '重置额度失败',
    rateLimitColumn: '速率限制',
    rateLimitSection: '速率限制',
    resetUsage: '重置',
    rateLimit5h: '5小时限额 (USD)',
    rateLimit1d: '日限额 (USD)',
    rateLimit7d: '7天限额 (USD)',
    rateLimitHint: '设置此密钥在指定时间窗口内的最大消费额。0 = 无限制。',
    rateLimitUsage: '速率限制用量',
    resetRateLimitUsage: '重置速率限制用量',
    resetRateLimitTitle: '确认重置速率限制',
    resetRateLimitConfirmMessage: '确定要重置密钥 "{name}" 的速率限制用量吗？所有时间窗口的已用额度将归零。此操作不可撤销。',
    rateLimitResetSuccess: '速率限制已重置',
    failedToResetRateLimit: '重置速率限制失败',
    resetNow: '即将重置',
    expiration: '密钥有效期',
    expiresInDays: '{days} 天',
    extendDays: '+{days} 天',
    customDate: '自定义',
    expirationDate: '过期时间',
    expirationDateHint: '选择此 API 密钥的过期时间。',
    currentExpiration: '当前过期时间',
    expiresAt: '过期时间',
    noExpiration: '永久有效',
    status: {
      active: '活跃',
      inactive: '已停用',
      quota_exhausted: '额度耗尽',
      expired: '已过期'
    }
  },

  // Usage
  usage: {
    title: '使用记录',
    description: '查看和分析您的 API 使用历史',
    costDetails: '费用明细',
    tokenDetails: 'Token 明细',
    cacheTtlOverriddenHint: '缓存 TTL Override 已启用',
    cacheTtlOverriddenLabel: 'TTL 替换',
    cacheTtlOverridden5m: '按 5m 计费',
    cacheTtlOverridden1h: '按 1h 计费',
    totalRequests: '总请求数',
    totalTokens: '总 Token',
    cacheTotal: '缓存',
    cacheBreakdown: '缓存 Token 明细',
    cacheCreationTokensLabel: '缓存创建',
    cacheReadTokensLabel: '缓存读取',
    totalCost: '总消费',
    standardCost: '标准',
    actualCost: '实际',
    accountCost: '成本',
    userBilled: '用户扣费',
    accountBilled: '账号计费',
    resetNow: '现在',
    resetPending: '待刷新',
    accountMultiplier: '账号倍率',
    avgDuration: '平均耗时',
    inSelectedRange: '所选范围内',
    perRequest: '每次请求',
    apiKeyFilter: 'API 密钥',
    allApiKeys: '全部密钥',
    timeRange: '时间范围',
    exportCsv: '导出 CSV',
    exportExcel: '导出 Excel',
    exportingProgress: '正在导出数据...',
    exportedCount: '已导出 {current}/{total} 条',
    estimatedTime: '预计剩余时间：{time}',
    cancelExport: '取消导出',
    exportCancelled: '导出已取消',
    exporting: '导出中...',
    preparingExport: '正在准备导出...',
    model: '模型',
    requestedModel: '请求',
    upstreamModel: '上游',
	  sentUpstreamModel: '发往上游',
	  upstreamResponseModel: '上游响应',
	  upstreamModelMismatch: '上游响应模型不一致',
	  modelVariant: '疑似版本变体',
	  modelMismatch: '模型不一致',
    reasoningEffort: '推理强度',
    requestedReasoningEffort: '请求推理强度',
    endpoint: '端点',
    endpointDistribution: '端点分布',
    inbound: '入站',
    upstream: '上游',
    mapping: '映射',
    path: '路径',
    inboundEndpoint: '入站端点',
    upstreamEndpoint: '上游端点',
    type: '类型',
    tokens: 'Token',
    cost: '费用',
    firstToken: '首 Token',
    duration: '耗时',
    latency: '延迟',
    latencyFirstToken: '首字',
    latencyDuration: '总耗时',
    time: '时间',
    ws: 'WS',
    stream: '流式',
    sync: '同步',
    nativeCompactionV2: '压缩',
    compactionFilter: '请求类别',
    allCompactionTypes: '全部请求',
    compactionOnly: '仅原生压缩',
    cyber: '安全策略',
    live: 'Live',
    unknown: '未知',
    in: '输入',
    out: '输出',
    cacheHit: '缓存命中',
    cacheCreate: '缓存创建',
    cacheHitRate: '缓存命中率',
    inputTokenPrice: '输入单价',
    outputTokenPrice: '输出单价',
    perMillionTokens: '/ 1M Token',
    unitPrice: '单次价格',
    imageUnitPrice: '单张价格',
    imageTotalPrice: '图片总价',
    imageCount: '图片张数',
    imageBillingSize: '计费尺寸',
    imageInputSize: '输入尺寸',
    imageOutputSize: '输出尺寸',
    imageInputTokens: '图片输入 Token',
    imageInputTokenPrice: '图片输入单价',
    imageInputCost: '图片输入费用',
    imageOutputTokens: '图片输出 Token',
    imageOutputTokenPrice: '图片输出单价',
    imageOutputCost: '图片输出费用',
    imageSizeSource: '尺寸来源',
    imageSizeBreakdown: '尺寸明细',
    imageSizeSourceOutput: '上游输出',
    imageSizeSourceInput: '请求输入',
    imageSizeSourceDefault: '默认计费档位',
    imageSizeSourceLegacy: '历史记录',
    imageSizeSourceMissing: '未记录',
    imageSizeNotRecorded: '未记录',
    imageSizeLegacyUnstandardized: '历史非标准',
    imageSizeUnknown: '未知',
    cacheRead: '读取',
    cacheWrite: '写入',
    serviceTier: '服务档位',
    serviceTierPriority: 'Fast',
    serviceTierUltrafast: 'Ultrafast',
    serviceTierFlex: 'Flex',
    serviceTierStandard: 'Standard',
    rate: '倍率',
    original: '原始',
    billed: '计费',
    noRecords: '未找到使用记录，请尝试调整筛选条件。',
    failedToLoad: '加载使用记录失败',
    noDataToExport: '没有可导出的数据',
    exportSuccess: '使用数据导出成功',
    exportFailed: '使用数据导出失败',
    exportExcelSuccess: '使用数据导出成功（Excel格式）',
    exportExcelFailed: '使用数据导出失败',
    imageUnit: '张',
    userAgent: 'User-Agent',
    ipGeo: {
      fetch: '获取地区',
      fetching: '获取中...',
      failed: '获取失败',
      private: '内网地址',
      refreshTitle: '刷新地区信息',
      batchFetch: '批量获取地区',
      batchFetching: '获取中...',
      pending: '{count} 个 IP 待获取地区',
      batchFailed: '批量获取地区信息失败',
      detailOrg: '运营商',
      detailTimezone: '时区',
      detailAccuracy: '定位精度',
      detailCoordinates: '坐标',
    },
    tabs: { usage: '用量明细', errors: '错误请求', ranking: '用户排行' },
    errors: {
      time: '时间', model: '模型', endpoint: '端点', status: '状态码',
      category: '分类', platform: '平台', message: '错误信息',
      keyName: 'Key 名称', keyDeleted: '已删除', allKeys: '全部 Key',
      modelPlaceholder: '搜索模型', allCategories: '全部分类', allStatuses: '全部状态码',
      empty: '暂无错误请求', failedToLoad: '加载错误请求失败',
      categories: {
        auth: '认证失败', rate_limit: '限流', quota: '余额/订阅',
        invalid_request: '参数错误', service_unavailable: '服务暂时不可用',
        upstream: '上游错误', internal: '平台错误', other: '其他', cyber: '安全策略',
      },
      detail: {
        title: '错误请求详情',
        responseBody: '上游响应内容',
        upstreamStatus: '上游状态码',
        loadFailed: '加载详情失败，请稍后重试',
      },
    },
  },

  // Shared keys for channel monitor (admin + user views)
  monitorCommon: {
    status: {
      operational: '正常',
      degraded: '降级',
      failed: '失败',
      error: '错误',
      unknown: '-'
    },
    providers: {
      openai: 'OpenAI',
      anthropic: 'Anthropic',
      gemini: 'Gemini',
      grok: 'Grok',
      antigravity: 'Antigravity',
      kimi: 'Kimi',
      zhipu: '智谱 GLM',
      deepseek: 'DeepSeek',
      minimax: 'MiniMax'
    },
    // 检查模式（监控条目的工作方式）
    checkMode: {
      probe: '探活',
      quota: '配额',
      quota_probe: '探活 + 配额'
    },
    // 配额快照展示（MonitorQuotaView，管理端与用户端共用）
    quota: {
      unavailable: '配额信息不可用',
      windows: {
        '5h': '5 小时',
        '7d': '7 天',
        '7dSonnet': '7 天 Sonnet',
        '7dFable': '7 天 Fable',
        weekly: '周',
        daily: '日',
        '30d': '30 天',
        total: '总量'
      },
      labels: {
        requests: '请求',
        tokens: 'Token',
        shared: '共享',
        pro: 'Pro',
        flash: 'Flash'
      }
    },
    extraModelsHeader: '附加模型',
    extraModelsEmpty: '无附加模型',
    latencyEmpty: '-',
    availabilityPrefix: '可用性',
    dialogLatency: '对话延迟',
    endpointPing: '端点 PING',
    history60pts: '近 {n} 次记录',
    nextUpdateIn: '{n}s 后刷新',
    past: 'PAST',
    now: 'NOW',
    maintenancePaused: '维护中 · 已暂停时间线采集',
    extraModelsCount: '+ {n} 模型',
    pollEvery: '{n}s 轮询',
    updatedAt: '更新于 {time}',
    relativeSecondsAgo: '{n} 秒前',
    relativeMinutesAgo: '{n} 分钟前',
    relativeHoursAgo: '{n} 小时前',
    relativeDaysAgo: '{n} 天前'
  },

  // Channel Status (user-facing read-only view)
  channelStatus: {
    title: '渠道状态',
    description: '查看渠道可用性、延迟和近期状态',
    searchPlaceholder: '搜索渠道...',
    allProviders: '全部供应商',
    loadError: '加载渠道状态失败',
    detailLoadError: '加载渠道详情失败',
    detailTitle: '渠道详情',
    closeDetail: '关闭',
    windowTab: {
      '7d': '7 天',
      '15d': '15 天',
      '30d': '30 天'
    },
    overall: {
      operational: 'OPERATIONAL',
      degraded: 'DEGRADED',
      unavailable: 'UNAVAILABLE'
    },
    columns: {
      name: '名称',
      provider: '供应商',
      groupName: '分组',
      primaryModel: '主模型',
      availability7d: '7 天可用率',
      latency: '延迟 (ms)'
    },
    detailColumns: {
      model: '模型',
      latestStatus: '最新状态',
      latestLatency: '最新延迟 (ms)',
      availability7d: '7 天可用率',
      availability15d: '15 天可用率',
      availability30d: '30 天可用率',
      avgLatency7d: '7 天平均延迟 (ms)'
    },
    empty: {
      title: '暂无可显示的渠道',
      description: '管理员尚未配置可监控的渠道。'
    }
  },

  // Available Channels (user-facing)
  availableChannels: {
    title: '可用渠道',
    description: '查看您可访问的渠道与其支持的模型、定价',
    searchPlaceholder: '搜索渠道或模型...',
    empty: '暂无可用渠道',
    noModels: '未配置模型',
    noPricing: '未配置定价',
    exclusive: '专属',
    public: '公开',
    exclusiveTooltip: '管理员授权给你的专属分组',
    publicTooltip: '对所有用户公开的分组',
    columns: {
      name: '渠道名',
      description: '描述',
      platform: '平台',
      groups: '我可访问的分组',
      supportedModels: '支持模型'
    },
    pricing: {
      billingMode: '计费模式',
      billingModeToken: '按 Token',
      billingModePerRequest: '按次',
      billingModeImage: '按图片',
      billingModeVideo: '按视频',
      inputPrice: '输入',
      outputPrice: '输出',
      cacheWritePrice: '缓存写入',
      cacheWrite5mPrice: '缓存写入（5m）',
      cacheWrite1hPrice: '缓存写入（1h）',
      cacheReadPrice: '缓存读取',
      imageInputPrice: '图片输入',
      imageOutputPrice: '图片输出',
      perRequestPrice: '每次请求',
      intervals: '阶梯定价',
      unitPerMillion: '/ 1M token',
      unitPerRequest: '/ 次'
    }
  },

  // Model Plaza (public group/model pricing showcase)
  modelPlaza: {
    title: '模型广场',
    description: '按分组浏览可用模型与价格',
    loading: '加载中...',
    empty: '暂无可展示的分组',
    loadFailed: '加载模型广场失败',
    noSearchResult: '没有匹配的模型',
    anonymousHint: '登录后可查看你的专属分组与专属倍率',
    filters: {
      platformLabel: '平台',
      groupLabel: '分组',
      rateLabel: '倍率',
      modelLabel: '模型',
      searchPlaceholder: '搜索模型名称',
      all: '全部'
    },
    badges: {
      exclusive: '专属分组',
      subscription: '订阅'
    },
    detail: {
      noModels: '该分组暂未配置模型',
      noPricing: '未配置定价',
      peakNote: '高峰时段 {window} 计费倍率 ×{multiplier}',
      longContextDisabledNote: '该分组未启用长上下文阶梯计费，超阈值请求仍按基础档计费，官方阶梯仅供参考'
    },
    table: {
      model: '模型',
      input: '输入',
      output: '输出',
      cache: '缓存',
      cacheWrite: '写入',
      cacheRead: '读取',
      cacheWriteShort: '写',
      cacheReadShort: '读',
      tierHint: '按单次请求的总上下文（输入 + 缓存写入 + 缓存读取）所在档位对整单计价',
      tierHintMarginal: '仅超过阈值的部分按该档计价，输出不加价',
      maxReasoningMultiplierBadge: 'Max ×{multiplier}',
      maxReasoningMultiplierHint: '最终转发的推理强度为 max 时，整次请求的计费与额度消耗乘以 {multiplier}',
      marginalBadge: '超出部分计价',
      timePricingRowHint: '按 {timezone} 时间，在该时段内发起的请求按本行价格计费',
      timePricingRowHintWeekdays:
        '按 {timezone} 时间，仅工作日（周一至周五）在该时段内发起的请求按本行价格计费，周末全天按标准价',
      timePricingRowHintPeak: '；本行价格未含高峰倍率，与高峰时段 {window} 重叠的部分实付再乘 ×{multiplier}',
      timePricingWeekdays: '工作日',
      timePricingRateHint: '生效倍率 {rate} × 时段倍率 {multiplier}',
      paidPrice: '实付价格(折后)',
      officialPrice: '官方价格',
      rate: '折扣倍率',
      unitPerMillion: '$ / 1M token',
      perUnitRequest: '/ 次',
      perUnitImage: '/ 张',
      perRequest: '按次计费',
      perImage: '按图片计费'
    },
    nav: {
      login: '登录',
      backToDashboard: '回到后台'
    }
  },

  affiliate: {
    title: '邀请返利',
    description: '邀请新用户注册，并将返利额度转入账户余额',
    yourCode: '我的邀请码',
    inviteLink: '邀请链接',
    copyCode: '复制邀请码',
    copyLink: '复制链接',
    codeCopied: '邀请码已复制',
    linkCopied: '邀请链接已复制',
    loadFailed: '加载邀请返利数据失败',
    transferFailed: '转入余额失败',
    stats: {
      rebateRate: '我的返利比例',
      rebateRateHint: '被邀请用户每次充值后你可获得的返利比例',
      invitedUsers: '邀请人数',
      availableQuota: '可转返利额度',
      frozenQuota: '冻结中',
      frozenQuotaHint: '新产生的返利正在冻结期中',
      totalQuota: '历史返利额度'
    },
    transfer: {
      title: '返利额度转余额',
      description: '将当前可用返利额度一键转入账户余额',
      button: '转入余额',
      transferring: '转入中...',
      empty: '当前没有可转入额度',
      success: '已转入余额：{amount}'
    },
    invitees: {
      title: '已邀请用户',
      empty: '暂无邀请记录',
      columns: {
        email: '邮箱',
        username: '用户名',
        rebate: '返利明细',
        joinedAt: '注册时间'
      }
    },
    tips: {
      title: '使用说明',
      line1: '将邀请码或邀请链接分享给新用户。',
      line2: '被邀请用户充值后，你可获得 {rate} 的返利额度。',
      line3: '返利额度可随时转入账户余额。',
      line4: '新产生的返利需要经过冻结期后才能提现。'
    }
  },

  // Redeem
  redeem: {
    title: '兑换码',
    description: '输入兑换码以充值余额或增加并发数',
    currentBalance: '当前余额',
    concurrency: '并发数',
    requests: '请求',
    redeemCodeLabel: '兑换码',
    redeemCodePlaceholder: '请输入兑换码',
    redeemCodeHint: '兑换码区分大小写',
    redeeming: '兑换中...',
    redeemButton: '兑换',
    redeemSuccess: '兑换成功！',
    redeemFailed: '兑换失败',
    added: '已添加',
    concurrentRequests: '并发请求',
    newBalance: '新余额',
    newConcurrency: '新并发数',
    aboutCodes: '关于兑换码',
    codeRule1: '每个兑换码只能使用一次',
    codeRule2: '兑换码可以增加余额、并发数或试用权限',
    codeRule3: '如有兑换问题，请联系客服',
    codeRule4: '余额和并发数即时更新',
    recentActivity: '最近活动',
    historyWillAppear: '您的兑换历史将显示在这里',
    balanceAddedRedeem: '余额充值（兑换）',
    balanceAddedAffiliate: '余额充值（返利转入）',
    balanceAddedAdmin: '余额充值（管理员）',
    balanceDeductedAdmin: '余额扣除（管理员）',
    concurrencyAddedRedeem: '并发增加（兑换）',
    concurrencyAddedAdmin: '并发增加（管理员）',
    concurrencyReducedAdmin: '并发减少（管理员）',
    adminAdjustment: '管理员调整',
    subscriptionAssigned: '订阅已分配',
    subscriptionAssignedDesc: '您已获得 {groupName} 的访问权限',
    subscriptionDays: '{days} 天',
    days: '天',
    codeRedeemSuccess: '兑换成功！',
    failedToRedeem: '兑换失败，请检查兑换码后重试。',
    subscriptionRefreshFailed: '兑换成功，但订阅状态刷新失败。',
    pleaseEnterCode: '请输入兑换码'
  },

  // Profile
  profile: {
    title: '个人设置',
    description: '管理您的账户信息和设置',
    accountBalance: '账户余额',
    concurrencyLimit: '并发限制',
    rpmLimit: 'RPM 限制',
    rpmUnlimited: '不限制',
    memberSince: '注册时间',
    overviewTitle: '账户总览',
    overviewDescription: '快速查看账号状态、资料来源与常用设置。',
    basicsTitle: '资料与头像',
    basicsDescription: '维护公开展示信息，并保持头像与昵称风格一致。',
    linkedProfileSources: '资料来源',
    linkedProfileSourcesDescription: '部分头像和昵称可能同步自第三方登录方式。',
    securityTitle: '安全设置',
    securityDescription: '密码、双因素认证和通知提醒集中放在右侧。',
    administrator: '管理员',
    user: '用户',
    username: '用户名',
    email: '邮箱',
    status: '状态',
    role: '角色',
    enterUsername: '输入用户名',
    editProfile: '编辑个人资料',
    updateProfile: '更新资料',
    updating: '更新中...',
    updateSuccess: '资料更新成功',
    updateFailed: '资料更新失败',
    usernameRequired: '用户名不能为空',
    leaderboardNamedParticipation: '在排行榜显示我的昵称',
    leaderboardNamedParticipationHint: '默认开启。关闭后你在排行榜上显示为「第 N 位」，仍然参与排名。需要昵称通过校验（2–32 个字符，不能是邮箱形态，不能含保留词）才能显示。',
    leaderboardNamedParticipationRejected: '当前昵称不符合排行榜的展示要求，无法开启。',
    changePassword: '修改密码',
    currentPassword: '当前密码',
    newPassword: '新密码',
    confirmNewPassword: '确认新密码',
    passwordHint: '密码至少需要 8 个字符',
    changingPassword: '修改中...',
    changePasswordButton: '修改密码',
    passwordsNotMatch: '两次输入的密码不一致',
    passwordTooShort: '密码至少需要 8 个字符',
    passwordChangeSuccess: '密码修改成功',
    passwordChangeFailed: '密码修改失败',
    // TOTP 2FA
    totp: {
      title: '双因素认证 (2FA)',
      description: '使用 Google Authenticator 等应用增强账户安全',
      enabled: '已启用',
      enabledAt: '启用时间',
      notEnabled: '未启用',
      notEnabledHint: '启用双因素认证可以增强账户安全性',
      enable: '启用',
      disable: '禁用',
      featureDisabled: '功能未开放',
      featureDisabledHint: '管理员尚未开放双因素认证功能',
      setupTitle: '设置双因素认证',
      setupStep1: '使用认证器应用扫描下方二维码',
      setupStep2: '输入应用显示的 6 位验证码',
      manualEntry: '无法扫码？手动输入密钥：',
      enterCode: '输入 6 位验证码',
      verify: '验证',
      setupFailed: '获取设置信息失败',
      verifyFailed: '验证码错误，请重试',
      enableSuccess: '双因素认证已启用',
      disableTitle: '禁用双因素认证',
      disableWarning: '禁用后，登录时将不再需要验证码。这可能会降低您的账户安全性。',
      enterPassword: '请输入当前密码确认',
      confirmDisable: '确认禁用',
      disableSuccess: '双因素认证已禁用',
      disableFailed: '禁用失败，请检查密码是否正确',
      loginTitle: '双因素认证',
      loginHint: '请输入您认证器应用显示的 6 位验证码',
      loginFailed: '验证失败，请重试',
      // New translations for email verification
      verifyEmailFirst: '请先验证您的邮箱',
      verifyPasswordFirst: '请先验证您的身份',
      emailCode: '邮箱验证码',
      enterEmailCode: '请输入 6 位验证码',
      sendCode: '发送验证码',
      codeSent: '验证码已发送到您的邮箱',
      sendCodeFailed: '发送验证码失败'
    },
    passkey: {
      title: 'Passkey',
      description: '使用面容 ID、触控 ID、Windows Hello 或安全密钥免密码登录。',
      add: '添加 Passkey',
      continue: '创建 Passkey',
      name: 'Passkey 名称',
      namePlaceholder: '例如：MacBook 触控 ID',
      passwordPlaceholder: '输入当前登录密码以确认',
      empty: '尚未添加任何 Passkey。',
      synced: '已同步',
      createdAt: '创建于 {date}',
      lastUsed: '上次使用 {date}',
      featureDisabled: '管理员尚未配置 Passkey 功能。',
      unsupported: '当前浏览器或设备不支持 Passkey。',
      loadFailed: '加载 Passkey 失败。',
      added: 'Passkey 已添加。',
      addFailed: '添加 Passkey 失败。',
      renamePrompt: '请输入新的 Passkey 名称',
      renamed: 'Passkey 已重命名。',
      renameFailed: '重命名 Passkey 失败。',
      deleteTitle: '删除 Passkey',
      deleteConfirm: '删除“{name}”？删除后将无法再使用它登录。',
      deleted: 'Passkey 已删除。',
      deleteFailed: '删除 Passkey 失败。'
    },
    balanceNotify: {
      title: '余额不足提醒',
      description: '当账户余额低于阈值时发送邮件提醒',
      enabled: '启用余额不足提醒',
      threshold: '自定义提醒阈值',
      thresholdHint: '留空使用系统默认值',
      thresholdPlaceholder: '输入金额',
      systemDefault: '系统默认值',
      extraEmails: '通知邮箱',
      extraEmailsHint: '必须添加并验证邮箱后，余额不足时才能收到提醒邮件',
      primaryEmail: '主邮箱',
      noExtraEmails: '暂无额外通知邮箱',
      enterEmail: '输入邮箱地址',
      addEmail: '添加邮箱',
      emailPlaceholder: '输入邮箱地址',
      sendCode: '发送验证码',
      resend: '重发',
      codeSent: '验证码已发送',
      codeSentTo: '验证码已发送到 {email}',
      enterCode: '输入验证码',
      codePlaceholder: '6位验证码',
      verify: '验证',
      emailAdded: '邮箱已添加',
      emailRemoved: '邮箱已移除',
      verifySuccess: '邮箱添加成功',
      removeEmail: '移除',
      removeSuccess: '邮箱已移除',
      emailDuplicate: '该邮箱已存在',
      maxEmailsReached: '已达到通知邮箱数量上限',
      unverified: '未验证',
      verified: '已验证',
    },
    avatar: {
      title: '资料头像',
      description: '仅支持上传头像图片；静态图片会自动压缩到 20KB 以内后再保存。',
      uploadAction: '上传图片',
      uploadHint: '上传图片时会自动压缩静态图片到 20KB 以内，GIF 需自行控制在 20KB 以内',
      uploadRequired: '请先上传头像图片',
      saveSuccess: '头像已更新',
      deleteSuccess: '头像已删除',
      invalidType: '请选择图片文件',
      gifTooLarge: 'GIF 头像必须在 20KB 以内',
      compressTooLarge: '无法将图片压缩到 20KB 以内，请换一张更小的图片',
      compressFailed: '压缩所选图片失败',
      readFailed: '读取所选图片失败',
      emptyDeleteHint: '当前没有可删除的头像',
    },
    authBindings: {
      title: '登录方式绑定',
      description: '查看当前绑定状态，并将更多第三方登录方式关联到这个账号。',
      bindAction: '绑定 {providerName}',
      bindSuccess: '账号绑定成功',
      emailPlaceholder: '输入邮箱地址',
      codePlaceholder: '输入验证码',
      passwordPlaceholder: '设置登录密码',
      replaceEmailPasswordPlaceholder: '输入当前密码',
      sendCodeAction: '发送验证码',
      manageEmailAction: '管理邮箱',
      hideEmailFormAction: '收起邮箱表单',
      confirmEmailBindAction: '绑定邮箱',
      confirmEmailReplaceAction: '更换主邮箱',
      codeSentTo: '验证码已发送到 {email}',
      replaceSuccess: '主邮箱已更新',
      unbindAction: '解绑',
      unbindSuccess: '{providerName} 已解绑',
      boundCount: '已关联 {count} 条记录',
      status: {
        bound: '已绑定',
        notBound: '未绑定',
      },
      providers: {
        email: '邮箱',
        linuxdo: 'LinuxDo',
        dingtalk: '钉钉',
        oidc: '{providerName}',
        wechat: '微信',
      },
      notes: {
        emailManagedFromProfile: '主邮箱在资料表单中管理',
        canUnbind: '你可以解绑这个登录方式。',
        bindAnotherBeforeUnbind: '请先绑定其他登录方式，再解除当前绑定。',
      },
      source: {
        avatar: '头像当前来自 {providerName}',
        username: '昵称当前来自 {providerName}',
      },
    }
  },

  // User Leaderboard(用量排行榜)
  leaderboard: {
    title: '用量排行榜',
    description: '按今日、本周和本月三个时间窗口，展示所有用户的用量与消费排名。榜单不展示邮箱地址。',
    windows: {
      label: '窗口',
      today: '今日',
      week: '本周',
      month: '本月',
    },
    metrics: {
      label: '排名指标',
      totalTokens: '总 Token',
      successfulRequests: '成功请求数',
      cost: '消费金额',
    },
    // 页头：H1 + 副题 + 一行控制条（窗口分段、指标分段、快照 chip、匿名档 chip）。
    // 日期进副题、时区进页脚、快照时分进 chip，因此这里不再有 window / metric / mode / tz /
    // snapshot 这些键名。窗口 / 指标只在页头一处，状态由页面持有。
    // 页面是套 AppLayout 的应用内页，侧边栏就是导航，因此没有站点标识与返回入口：
    // `brand`、`backToDashboard` 与三个主题开关键都已删除（零引用）。
    masthead: {
      // 档位 chip 只在匿名档出现；实名档是常态，不挂任何档位 chip。
      // 快照 chip 与页脚 colophon 共用的标签；快照还没生成时显示 snapshotPending。
      snapshot: '快照',
      snapshotPending: '待生成',
      modeAnonymous: '匿名档',
      // 「距下一次重建还剩几分钟」，按重建周期现算；快照未生成或已陈旧时整段不渲染。
      rebuildIn: '（{minutes} 分钟后重建）',
      metricTokens: 'Token',
      metricRequests: '请求数',
      metricCost: '金额',
    },
    // 标题块（v3 取代 v2 的命令行标题）。`leaderboard.title` 已经是路由标题用的叶子字符串
    // （router/index.ts 的 titleKey），因此这里另起 titleBlock，MUST NOT 把 title 改成对象。
    titleBlock: {
      heading: {
        today: '今日概览',
        week: '本周概览',
        month: '本月概览',
      },
      participantsUnit: '位活跃',
    },
    // 七章的章号是固定编号而不是序号：某一章整章不渲染时其余章号不重排。
    // 键名里的 `01`–`07` 就是页面上印的那个章号。章名右侧的小字副题已整体去掉，因此没有 `sub`。
    // 只有 `03`（亮点）的章名随 Window 变，是三个变体的对象，其余六章都是字符串。
    chapters: {
      '01': {
        name: '排行榜（前 50 名）',
      },
      '02': {
        name: '你的排名',
      },
      '03': {
        name: {
          today: '今日亮点',
          week: '本周亮点',
          month: '本月亮点',
        },
      },
      '04': {
        name: '六项纪录',
      },
      '05': {
        name: '模型与平台',
      },
      '06': {
        name: '活跃节奏',
      },
      '07': {
        name: '趋势与构成',
      },
    },
    extremes: {
      nightOwl: {
        label: '深夜活跃',
        unit: '0–6 点占其自身用量',
      },
      rising: {
        label: '增长之星',
        unit: '较昨日',
      },
      omnivore: {
        label: '多模型用户',
        unit: '种不同模型',
      },
      talker: {
        label: '输出占比最高',
        unit: '输出 token 占比',
      },
      maxSingle: {
        label: '单次请求峰值',
        unit: 'tokens / 次',
        ratioUnit: '倍于中位数',
        medianNote: '中位数的 {ratio} 倍',
      },
      streak: {
        label: '连续活跃',
        unit: '天不间断',
      },
    },
    whoami: {
      // 全页只留这一处「仅本人可见」标记，章名旁不再重复一遍。
      note: '仅本人可见',
      // 窗口长度由折线实际拿到的点数现算，MUST NOT 写死 14
      rankTrend: '近 {span} 天排名走势',
      models: '常用模型',
      compare: '与全站对比',
      cacheHitRate: '缓存命中率',
      avgTokens: '单请求平均 tokens',
      siteValue: '全站 {value}',
      // v3 报表皮肤新增
      rankEyebrow: '当前名次',
      rankSummary: '最佳 #{best} · 最差 #{worst}',
      meLabel: '我',
      siteLabel: '全站',
    },
    profiles: {
      // 每人列出该窗口用过的全部模型；占比向下取整，不足 1% 的显示为 <1%
      note: '模型偏好 · 按成功请求占比',
    },
    platforms: {
      note: '今日请求按平台',
      // 图例右侧的小字：实名档是成功请求数，匿名档整段缺席。
      legendCount: '{count} 次',
      // 最后一项是各项占比取整后的残差，不是一个平台，因此 MUST NOT 为它编一个请求数，
      // 也不再为它挂一句「取整残差」的解释。
      other: '其他',
    },
    rhythm: {
      note: '周内节奏 · 近 4 周',
      weekdays: {
        mon: '周一',
        tue: '周二',
        wed: '周三',
        thu: '周四',
        fri: '周五',
        sat: '周六',
        sun: '周日',
      },
    },
    composition: {
      note: '今日四类 tokens 构成',
      input: '输入',
      output: '输出',
      cacheCreation: '缓存创建',
      cacheRead: '缓存读取',
    },
    // `$ cache --trend 14` 是页面上唯一的缓存区块：大号数字取今日命中率，
    // 说明行按档位二选一，折线是近 14 天。
    cacheTrend: {
      note: '全站命中率 · 近 14 天',
      sub: '缓存读取占输入 {rate}%',
      subNamed: '{hits} tokens 命中缓存 · 输入合计 {inputs}',
      // 有趋势时大号数字下面只是一个标签：区间由右边的折线自己说。
      range: '今日',
    },
    highlights: {
      topTokens: {
        today: '今日用量最高',
        week: '本周用量最高',
        month: '本月用量最高',
      },
      cacheKing: '缓存效率最高',
      topRequests: '成功请求最多',
      site: {
        today: '全站今日',
        week: '全站本周',
        month: '全站本月',
      },
      leadPercent: '领先 {percent}%',
      dominantModel: '主要模型：{model}',
      // v3 报表皮肤：大数字与单位分开，句子由前端按档位现算（缺数据就少一句分句）。
      topTokensEyebrow: '{label} · tokens 用量最高',
      cacheKingEyebrow: '{label} · 缓存命中率最高',
      topRequestsEyebrow: '{label} · 成功请求最多',
      runnerUp: '第 2 名',
      unitTokens: 'tokens',
      unitShareTokens: '占全站 tokens',
      unitCacheHitRate: '缓存命中率',
      unitRequests: '成功请求',
      unitShareRequests: '占全站请求',
      leadSay: '领先第 2 名 {lead}% · 占全站 {share}%',
      leadSayTie: '与第 2 名并列 · 占全站 {share}%',
      shareOfSite: '占全站 {percent}%',
      tiedWithSecond: '与第 2 名并列',
      siteRows: {
        totalTokens: '总 Token',
        successfulRequests: '成功请求',
        participants: '活跃人数',
        peakHour: '峰值时段',
        cacheHitRate: '缓存命中率',
      },
      rowRequests: '{count} 次',
      rowParticipants: '{count} 人',
    },
    rank: {
      // 「用户排行」是渠道监控里那张诊断表的名字（见 CONTEXT.md 的 Avoid 列表），这里不借用。
      // 表下只剩一行解读句，两个分句由前端从 entries 现算，缺数据的分句整句省略，
      // 两档同形（匿名档也只用倍数与占比）。分句不带句末标点，由前端用 ` · ` 连起来。
      readout: {
        lead: '第 1 名是第 2 名的 {ratio} 倍',
        topThreeShare: '前三名占全站 {percent}%',
      },
    },
    table: {
      rank: '名次',
      user: '用户',
      relativeToTop: '相对第一名',
      totalTokens: '总 Token',
      // 表头与「你的位置」那两个小标签用短词，Metric 分段仍用完整的「成功请求数」。
      successfulRequestsShort: '成功请求',
      // 金额列：实名档与 Preview 下是绝对金额（USD），匿名档他人行是相对第一名的百分比。
      cost: '金额',
      relativePercent: '第一名的 {percent}%',
      // 匿名档本人行：第一名的绝对量前端拿不到，这一格只能留占位符
      relativeUnknown: '匿名档下第一名的用量不公开，无法算出本行相对第一名的百分比',
      selfBadge: '你',
    },
    identity: {
      // 他人的假名用榜单序号（ordinal），不是 user_id；「第 N 位」读起来是名次口吻，
      // 与旧的「用户 #N」相比不会被误读成用户编号。
      anonymous: '第 {ordinal} 位',
      outOfRank: '榜外用户',
    },
    myRank: {
      participants: '共 {count} 人参与',
      noUsage: '本时间段暂无用量',
      noUsageHint: '产生用量后即可获得名次。',
      suppressedHint: '参与人数过少，为保护匿名性暂不展示榜单',
      hint: {
        gapTokens: '再增加 {gap} tokens 即可进入前 {rank}',
        gapRequests: '再增加 {gap} 次成功请求即可进入前 {rank}',
        gapCost: '再消费 {gap} 即可进入前 {rank}',
        relative: '你为第一名的 {percent}%，第 {rank} 名为 {target}%',
      },
    },
    insights: {
      modelHeat: {
        title: '今日模型',
      },
      heatmap: {
        title: '近 30 天活跃度',
        legendLow: '少',
        legendHigh: '多',
        cellRequests: '{date} · {count} 次请求',
        cellRelative: '{date} · 相对最高日 {percent}%',
      },
      trend: {
        title: '用量趋势 · 近 14 天',
        sub: '较前一日 {change}',
        monthTotal: '本月累计',
        monthChange: '较上月',
        // 断轴：取 14 天里第三大的值 T，最大值严格大于 5T 时纵轴在 T 处压缩。
        // 不满足条件时线性，这条标注不渲染；图下那句解释已删，标注本身就说明轴被压缩了。
        axisBreak: {
          label: '轴在 {value} 压缩',
        },
      },
      hourly: {
        title: '今日时段分布',
        peak: '峰值 {hour}:00',
        peakWithRequests: '峰值 {hour}:00 · {count} 请求',
        barRequests: '{hour}:00 · {count} 次请求',
        barRelative: '{hour}:00 · 相对峰值 {percent}%',
      },
    },
    // 页脚是一行 colophon：`snapshot HH:MM · 每 5 分钟重建 · <站点时区> · 不展示邮箱地址`。
    // `snapshot` 与时区名是字面量，不进 i18n（design D23）；`COLOPHON` 字样、
    // 「一周从周一起算」与 `successful_requests = actual_cost > 0` 已删。
    // 金额已是第三个 Metric（与 tokens 同一套档位规则），这一段因此只剩邮箱那一句；
    // 键名 `noMoney` 是 LbFooter.vue 引用的既有键，保持不变。
    footer: {
      rebuild: '每 5 分钟重建',
      noMoney: '不展示邮箱地址',
      // 年份与站名取自运行时，MUST NOT 写死。
      copyright: '© {year} {site}. All rights reserved.',
    },
    states: {
      loadFailed: '榜单加载失败，请稍后重试',
      empty: '当前时间段还没有用量记录',
      emptyHint: '产生第一笔用量后，榜单会自动出现。',
      suppressed: '参与人数不足，暂不展示榜单',
      computing: '正在生成榜单',
      computingHint: '榜单每 5 分钟更新一次，首次开启后请稍候。',
      stale: '榜单已超过 15 分钟未更新',
      staleHint: '当前显示的是上一版数据，后台恢复后会自动更新。',
    },
    preview: {
      banner: '预览模式：普通用户不可见',
      bannerHint: '排行榜模式当前为「关闭」，只有管理员能通过直链看到本页。开启后管理员与普通用户看到的数据完全相同。',
    },
  },

  // Empty States
  empty: {
    noData: '暂无数据'
  },

  // Table
  table: {
    expandActions: '展开更多操作',
    collapseActions: '收起操作'
  },

  // Pagination
  pagination: {
    showing: '显示',
    to: '至',
    of: '共',
    results: '条结果',
    page: '页',
    pageOf: '第 {page} / {total} 页',
    previous: '上一页',
    next: '下一页',
    perPage: '每页',
    goToPage: '跳转到第 {page} 页',
    jumpTo: '跳转页',
    jumpPlaceholder: '页码',
    jumpAction: '跳转'
  },

  // Errors
  errors: {
    somethingWentWrong: '出错了',
    pageNotFound: '页面未找到',
    unauthorized: '未授权',
    forbidden: '禁止访问',
    serverError: '服务器错误',
    networkError: '网络错误',
    timeout: '请求超时',
    tryAgain: '请重试'
  },

  // Dates
  dates: {
    today: '今天',
    yesterday: '昨天',
    thisWeek: '本周',
    lastWeek: '上周',
    thisMonth: '本月',
    lastMonth: '上月',
    last24Hours: '近24小时',
    last7Days: '近 7 天',
    last14Days: '近 14 天',
    last30Days: '近 30 天',
    custom: '自定义',
    startDate: '开始日期',
    endDate: '结束日期',
    apply: '应用',
    selectDateRange: '选择日期范围'
  },

  // Admin
}
