import {useQuery} from '@tanstack/react-query'
import {create} from 'zustand'
import {get} from '@/lib/api'
import type {WhatsNewStatus} from '@/lib/types'

// The What's new dialog opens by itself once per edition; the command palette
// and the rename banner reopen it so the rename check stays reachable after it
// was closed. The banner also starts the check right away.
export const useWhatsNew = create<{reopened: boolean; autoCheck: boolean}>(() => ({
  reopened: false,
  autoCheck: false,
}))
export const openWhatsNew = (autoCheck = false) => useWhatsNew.setState({reopened: true, autoCheck})

export const whatsNewKey = ['whats-new']
export const useWhatsNewStatus = () =>
  useQuery({queryKey: whatsNewKey, queryFn: () => get<WhatsNewStatus>('/api/whats-new')})
