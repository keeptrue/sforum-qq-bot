const zhCN = {
  'bind.title': '绑定 QQ 机器人',
  'bind.hint': '用手机 QQ 扫码，AppID 与 AppSecret 会自动填好并保存；也可以手动填写。',
  'bind.start': '扫码绑定',
  'bind.manual': '手动填写凭据',
  'bind.retry': '重新生成二维码',
  'bind.rebind': '重新绑定',
  'bind.starting': '正在生成二维码…',
  'bind.waiting': '用手机 QQ 扫码，然后在手机上确认授权。',
  'bind.expires': '{time} 后过期',
  'bind.openLink': '二维码扫不出来？在手机 QQ 中打开这个链接',
  'bind.saving': '扫码成功，正在保存凭据…',
  'bind.bound': '已绑定并保存',
  'bind.boundId': '当前 AppID：{id}',
  'bind.expired': '二维码已过期，请重新生成。',
  'bind.error': '绑定流程没有走通，请重试；也可以直接手动填写凭据。',

  'group.connection': '连接',
  'group.connection.hint': '机器人应用凭据。AppSecret 只写不读，留空表示保留已保存的值。',
  'group.targets': '推送目标',
  'group.targets.hint': '推给哪些群、哪些分类，以及什么时候停。',
  'group.template': '消息模板',
  'group.template.hint': '一条推送长什么样。改完保存，下一次推送就生效。',
  'group.limits': '限速与重试',
  'group.limits.hint': '插件侧的护栏，避免撞上 QQ 的频控与日额度。',
  'group.probe': '排错',
  'group.probe.hint': '排查问题时才需要打开，平时保持关闭。',
  'group.other': '其他设置',
  'group.other.hint': '清单里新增的分组。',

  'field.saved': '已保存',
  'field.keepSecret': '已保存，留空表示不修改',
  'field.secretPlaceholder': '填写 AppSecret',

  'template.tokens': '可用变量（点击插入到光标处）',
  'template.tokensHint': '中文别名同样有效，例如 {{标题}}。不认识的变量会原样保留，方便你对照检查。',
  'template.preview': '预览',
  'template.previewEmpty': '（模板为空，渲染结果也是空的）',
  'template.previewFailed': '预览暂时不可用；保存后推送仍会正常渲染。',
  'template.truncated': '已截断',
  'template.unknown': '未知变量',
  'template.count': '{n} 字符',
  'template.live': '预览已更新，{n} 字符',

  'footer.title': '保存与测试',
  'footer.hint': '测试消息与推送状态都按已保存的配置执行，不改设置、不写投递记录。',
  'footer.save': '保存',
  'footer.saveBusy': '正在保存…',
  'footer.reset': '恢复默认',
  'footer.resetArmed': '再点一次确认恢复',
  'footer.test': '发一条测试消息',
  'footer.testBusy': '正在发送…',
  'footer.status': '推送状态',
  'footer.statusBusy': '正在读取…',
  'footer.savedAt': '已提交保存 · {time}',
  'footer.resetDone': '已提交恢复默认 · {time}',
  'footer.testOk': '测试消息已发出',
  'footer.testFail': '测试消息没有发出',
  'footer.statusOk': '状态已刷新',
  'footer.statusFail': '读取状态失败',

  'error.routeTitle': '先在插件列表启用本插件，再回来绑定。',
  'error.routeDetail': '如果插件已经启用，请确认后端已随保存重启；原始错误不在这里展示。',
  'error.generic': '操作没有成功，请重试。',
  'error.saveFailed': '扫码成功，但自动保存没有生效，请检查后在页面底部手动保存。'
} as const

export type MessageKey = keyof typeof zhCN

const enUS: Record<MessageKey, string> = {
  'bind.title': 'Connect the QQ bot',
  'bind.hint': 'Scan with mobile QQ and AppID plus AppSecret are filled in and saved for you. You can also type them by hand.',
  'bind.start': 'Scan to connect',
  'bind.manual': 'Enter credentials by hand',
  'bind.retry': 'Generate a new code',
  'bind.rebind': 'Connect again',
  'bind.starting': 'Generating the QR code…',
  'bind.waiting': 'Scan the code with mobile QQ, then confirm on the phone.',
  'bind.expires': 'expires in {time}',
  'bind.openLink': 'Cannot scan it? Open this link in mobile QQ',
  'bind.saving': 'Scanned. Saving the credentials…',
  'bind.bound': 'Connected and saved',
  'bind.boundId': 'Current AppID: {id}',
  'bind.expired': 'The code expired. Generate a new one.',
  'bind.error': 'The connection flow did not go through. Try again, or enter the credentials by hand.',

  'group.connection': 'Connection',
  'group.connection.hint': 'Bot application credentials. The AppSecret is write-only; leave it blank to keep the saved value.',
  'group.targets': 'Delivery targets',
  'group.targets.hint': 'Which groups and categories get pushed, and when to stop.',
  'group.template': 'Message template',
  'group.template.hint': 'What one push looks like. Save and the next push uses it.',
  'group.limits': 'Rate limits and retries',
  'group.limits.hint': 'Plugin-side guardrails that keep you under the QQ rate limits and daily quota.',
  'group.probe': 'Troubleshooting',
  'group.probe.hint': 'Turn it on only while chasing a problem.',
  'group.other': 'Other settings',
  'group.other.hint': 'A group added to the manifest.',

  'field.saved': 'Saved',
  'field.keepSecret': 'Saved; leave blank to keep it',
  'field.secretPlaceholder': 'Enter the AppSecret',

  'template.tokens': 'Variables (click to insert at the cursor)',
  'template.tokensHint': 'Chinese aliases work too, e.g. {{标题}}. Unknown variables are left untouched so you can spot typos.',
  'template.preview': 'Preview',
  'template.previewEmpty': '(the template is empty, so the rendered message is empty)',
  'template.previewFailed': 'The preview is unavailable right now; pushes still render it.',
  'template.truncated': 'Truncated',
  'template.unknown': 'Unknown variables',
  'template.count': '{n} characters',
  'template.live': 'Preview updated, {n} characters',

  'footer.title': 'Save and test',
  'footer.hint': 'The test message and the status report both run against the saved settings; neither changes settings nor writes a delivery record.',
  'footer.save': 'Save',
  'footer.saveBusy': 'Saving…',
  'footer.reset': 'Restore defaults',
  'footer.resetArmed': 'Click again to confirm',
  'footer.test': 'Send a test message',
  'footer.testBusy': 'Sending…',
  'footer.status': 'Delivery status',
  'footer.statusBusy': 'Reading…',
  'footer.savedAt': 'Save submitted · {time}',
  'footer.resetDone': 'Restore submitted · {time}',
  'footer.testOk': 'The test message went out',
  'footer.testFail': 'The test message did not go out',
  'footer.statusOk': 'Status refreshed',
  'footer.statusFail': 'Could not read the status',

  'error.routeTitle': 'Enable this plugin in the plugin list, then come back.',
  'error.routeDetail': 'If the plugin is already enabled, check that the backend restarted after the last save; raw errors are not shown here.',
  'error.generic': 'That did not work. Please try again.',
  'error.saveFailed': 'Scanned, but the automatic save did not take effect. Check the form and save it at the bottom of the page.'
}

export type I18n = Readonly<{
  zh: boolean
  t: (key: MessageKey, params?: Record<string, string | number>) => string
}>

export function createI18n(locale: string): I18n {
  const zh = (locale || '').toLowerCase().startsWith('zh')
  const table: Record<MessageKey, string> = zh ? zhCN : enUS
  return {
    zh,
    t: (key, params) => {
      let text: string = table[key]
      if (params) {
        for (const [name, value] of Object.entries(params)) {
          text = text.replaceAll(`{${name}}`, String(value))
        }
      }
      return text
    }
  }
}
