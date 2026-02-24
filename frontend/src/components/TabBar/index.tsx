import { useEffect, useRef, useState } from 'react'
import { X, Plus, Zap, Radio, Pin, PinOff, Copy, ChevronDown } from 'lucide-react'
import { useTabStore } from '../../stores'
import type { RequestTab } from '../../stores'
import styles from './TabBar.module.css'

export function TabBar() {
  const { tabs, activeTabId, openNewTab, closeTab, setActiveTab } = useTabStore()
  const [contextMenu, setContextMenu] = useState<{ tabId: string; x: number; y: number } | null>(null)
  const [pinnedTabs, setPinnedTabs] = useState<Set<string>>(new Set())
  const menuRef = useRef<HTMLDivElement>(null)

  // Close context menu on outside click
  useEffect(() => {
    function handler(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setContextMenu(null)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  function togglePin(tabId: string) {
    setPinnedTabs(prev => {
      const next = new Set(prev)
      next.has(tabId) ? next.delete(tabId) : next.add(tabId)
      return next
    })
  }

  function duplicateTab(tabId: string) {
    const tab = tabs.find(t => t.id === tabId)
    if (!tab) return
    useTabStore.getState().openFromMethod(
      tab.service || '',
      tab.method || tab.title,
      tab.requestBody,
      tab.connectionId
    )
  }

  function closeOthers(tabId: string) {
    tabs.forEach(t => { if (t.id !== tabId && !pinnedTabs.has(t.id)) closeTab(t.id) })
  }

  function closeRight(tabId: string) {
    const idx = tabs.findIndex(t => t.id === tabId)
    tabs.slice(idx + 1).forEach(t => { if (!pinnedTabs.has(t.id)) closeTab(t.id) })
  }

  // Sorted: pinned first
  const sortedTabs = [
    ...tabs.filter(t => pinnedTabs.has(t.id)),
    ...tabs.filter(t => !pinnedTabs.has(t.id)),
  ]

  return (
    <>
      <div className={styles.tabBar}>
        <div className={styles.tabList}>
          {sortedTabs.map(tab => (
            <TabItem
              key={tab.id}
              tab={tab}
              isActive={tab.id === activeTabId}
              isPinned={pinnedTabs.has(tab.id)}
              onActivate={() => setActiveTab(tab.id)}
              onClose={(e) => {
                e.stopPropagation()
                if (!pinnedTabs.has(tab.id)) closeTab(tab.id)
              }}
              onContextMenu={(e) => {
                e.preventDefault()
                setContextMenu({ tabId: tab.id, x: e.clientX, y: e.clientY })
              }}
            />
          ))}
        </div>
        <button className={styles.newTabBtn} onClick={openNewTab} title="New tab (Ctrl+T)">
          <Plus size={13} />
        </button>
      </div>

      {/* Context menu */}
      {contextMenu && (
        <div
          ref={menuRef}
          className={styles.contextMenu}
          style={{ top: contextMenu.y, left: contextMenu.x }}
        >
          <button onClick={() => { togglePin(contextMenu.tabId); setContextMenu(null) }}>
            {pinnedTabs.has(contextMenu.tabId)
              ? <><PinOff size={12} /> Unpin tab</>
              : <><Pin size={12} /> Pin tab</>
            }
          </button>
          <button onClick={() => { duplicateTab(contextMenu.tabId); setContextMenu(null) }}>
            <Copy size={12} /> Duplicate tab
          </button>
          <div className={styles.contextDivider} />
          <button onClick={() => { closeOthers(contextMenu.tabId); setContextMenu(null) }}>
            Close other tabs
          </button>
          <button onClick={() => { closeRight(contextMenu.tabId); setContextMenu(null) }}>
            Close tabs to the right
          </button>
          <div className={styles.contextDivider} />
          <button className={styles.contextDanger}
            onClick={() => {
              if (!pinnedTabs.has(contextMenu.tabId)) closeTab(contextMenu.tabId)
              setContextMenu(null)
            }}>
            <X size={12} /> Close tab
          </button>
        </div>
      )}
    </>
  )
}

function TabItem({ tab, isActive, isPinned, onActivate, onClose, onContextMenu }: {
  tab: RequestTab
  isActive: boolean
  isPinned: boolean
  onActivate: () => void
  onClose: (e: React.MouseEvent) => void
  onContextMenu: (e: React.MouseEvent) => void
}) {
  const isStream = tab.method?.toLowerCase().includes('stream')

  return (
    <div
      className={`${styles.tab} ${isActive ? styles.tabActive : ''} ${isPinned ? styles.tabPinned : ''}`}
      onClick={onActivate}
      onContextMenu={onContextMenu}
      title={tab.service ? `${tab.service}/${tab.method}` : tab.title}
    >
      <span className={styles.tabIcon}>
        {isStream
          ? <Radio size={10} className={styles.iconStream} />
          : <Zap size={10} className={styles.iconUnary} />
        }
      </span>

      {isPinned && <Pin size={9} className={styles.pinIcon} />}

      <span className={styles.tabTitle}>
        {tab.title}
        {tab.isDirty && <span className={styles.dirty} />}
      </span>

      {tab.responseStatus && (
        <span className={`${styles.statusDot} ${tab.responseStatus === 'OK' ? styles.statusDotOk : styles.statusDotErr}`} />
      )}

      {!isPinned && (
        <button className={styles.closeBtn} onClick={onClose} title="Close">
          <X size={11} />
        </button>
      )}
    </div>
  )
}
