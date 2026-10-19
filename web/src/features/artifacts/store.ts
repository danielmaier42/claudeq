import {create} from 'zustand'

// The artifact viewer: open one artifact from anywhere (Artifacts, the run log,
// a notification click). ArtifactViewer.tsx is mounted once by the shell.
// `sequence` is the order ← and → step through: the Artifacts view hands in
// the list as filtered on screen; opened from elsewhere, the viewer steps
// through every artifact, newest first.
export const useArtifactViewer = create<{artifactId: string | null; sequence: string[] | null}>(() => ({
  artifactId: null,
  sequence: null,
}))

export const openArtifact = (artifactId: string, sequence?: string[]) =>
  useArtifactViewer.setState({artifactId, sequence: sequence ?? null})
export const closeArtifact = () => useArtifactViewer.setState({artifactId: null, sequence: null})
// stepArtifact moves the open viewer to another artifact of the same sequence.
export const stepArtifact = (artifactId: string) => useArtifactViewer.setState({artifactId})
