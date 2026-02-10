import { useState } from 'react'
import { useLanguage } from '../contexts/LanguageContext'
import { MarketCacheManager } from '../components/MarketCacheManager'

type Tab = 'dashboard' | 'cache'

export function DataPage() {
  const { language } = useLanguage()
  const [tab, setTab] = useState<Tab>('dashboard')

  return (
    <div className="w-full h-[calc(100vh-64px)] px-4 py-3 space-y-3">
      <div className="flex items-center gap-2">
        <button
          onClick={() => setTab('dashboard')}
          className={`px-3 py-2 rounded text-sm font-medium ${
            tab === 'dashboard'
              ? 'bg-[#0ECB81] text-black'
              : 'bg-[#1E2329] text-[#EAECEF] border border-[#2B3139]'
          }`}
        >
          {language === 'zh' ? '云端数据看板' : 'Cloud Dashboard'}
        </button>
        <button
          onClick={() => setTab('cache')}
          className={`px-3 py-2 rounded text-sm font-medium ${
            tab === 'cache'
              ? 'bg-[#0ECB81] text-black'
              : 'bg-[#1E2329] text-[#EAECEF] border border-[#2B3139]'
          }`}
        >
          {language === 'zh' ? '本地行情缓存' : 'Local Market Cache'}
        </button>
      </div>

      {tab === 'dashboard' ? (
        <div className="w-full h-[calc(100vh-110px)]">
          <iframe
            src="https://nofxos.ai/dashboard"
            title={language === 'zh' ? '数据中心' : 'Data Center'}
            className="w-full h-full border-0 rounded-lg overflow-hidden"
            allow="fullscreen"
          />
        </div>
      ) : (
        <div className="w-full h-[calc(100vh-110px)] overflow-auto">
          <MarketCacheManager />
        </div>
      )}
    </div>
  )
}
