import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export interface EnvVar {
  id: string
  envId: string
  key: string
  value: string
  secret: boolean
}

export interface Environment {
  id: string
  name: string
  color: string
  variables: EnvVar[]
  createdAt: string
  updatedAt: string
}

interface EnvironmentStore {
  environments: Environment[]
  activeEnvId: string | null

  // Actions
  setEnvironments: (envs: Environment[]) => void
  setActiveEnv: (id: string | null) => void
  upsertEnvironment: (env: Environment) => void
  removeEnvironment: (id: string) => void
  getActiveEnv: () => Environment | null
  resolveVariables: (text: string) => string
}

export const useEnvironmentStore = create<EnvironmentStore>()(
  persist(
    (set, get) => ({
      environments: [],
      activeEnvId: null,

      setEnvironments: (envs) => set({ environments: envs }),
      setActiveEnv: (id) => set({ activeEnvId: id }),

      upsertEnvironment: (env) => set((s) => {
        const idx = s.environments.findIndex(e => e.id === env.id)
        if (idx >= 0) {
          const updated = [...s.environments]
          updated[idx] = env
          return { environments: updated }
        }
        return { environments: [...s.environments, env] }
      }),

      removeEnvironment: (id) => set((s) => ({
        environments: s.environments.filter(e => e.id !== id),
        activeEnvId: s.activeEnvId === id ? null : s.activeEnvId,
      })),

      getActiveEnv: () => {
        const { environments, activeEnvId } = get()
        return environments.find(e => e.id === activeEnvId) ?? null
      },

      resolveVariables: (text: string) => {
        const env = get().getActiveEnv()
        if (!env) return text
        let result = text
        for (const v of env.variables) {
          result = result.split(`{{${v.key}}}`).join(v.value)
        }
        return result
      },
    }),
    {
      name: 'platr-env-active',
      partialize: (state) => ({ activeEnvId: state.activeEnvId }),
    }
  )
)
