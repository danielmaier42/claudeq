import {create} from 'zustand'

export const usePalette = create<{open: boolean}>(() => ({open: false}))
export const openPalette = () => usePalette.setState({open: true})
export const setPaletteOpen = (open: boolean) => usePalette.setState({open})
