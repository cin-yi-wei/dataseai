import { useLang, useT, LANGS } from '../i18n'

export default function LangSwitcher() {
  const lang = useLang((s) => s.lang)
  const setLang = useLang((s) => s.setLang)
  const t = useT()
  return (
    <select
      value={lang}
      onChange={(e) => setLang(e.target.value as any)}
      title={t('lang.label')}
      style={{ fontSize: 12, padding: '2px 4px' }}
    >
      {LANGS.map((l) => (
        <option key={l} value={l}>
          {l === 'zh-TW' ? t('lang.zh_tw') : t('lang.en')}
        </option>
      ))}
    </select>
  )
}
