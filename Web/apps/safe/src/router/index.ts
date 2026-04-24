import { createRouter, createWebHistory } from 'vue-router'
import setupClusterGuard from './guards/clusterReady'
import setupAuthGuard from './guards/auth'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'Login',
      component: () => import('../pages/Login/Login.vue'),
      meta: { hideMenu: true },
    },
    {
      path: '/login-admin',
      name: 'LoginAdmin',
      component: () => import('../pages/Login/Login.vue'),
    },
    {
      path: '/register',
      name: 'Register',
      component: () => import('../pages/Login/Register.vue'),
      meta: { hideMenu: true },
    },
    {
      path: '/sso-error',
      name: 'SSOError',
      component: () => import('../pages/Login/SSOError.vue'),
      meta: { hideMenu: true },
    },
    {
      path: '/error',
      name: 'ErrorPage',
      component: () => import('../pages/Error/ErrorPage.vue'),
    },
    {
      path: '/',
      component: () => import('@/layouts/AppLayout.vue'),
      children: [
        {
          path: '',
          name: 'Homepage',
          component: () => import('../pages/Homepage/index.vue'),
        },
        {
          path: '/nodes',
          name: 'Nodes',
          component: () => import('../pages/Nodes/index.vue'),
        },
        {
          path: '/nodedetail',
          name: 'NodeDetail',
          component: () => import('../pages/Nodes/NodeDetail.vue'),
        },
        {
          path: '/clusters',
          name: 'Clusters',
          component: () => import('../pages/Clusters/index.vue'),
        },
        {
          path: '/cluster/detail',
          name: 'clusterDetail',
          component: () => import('../pages/Clusters/ClusterDetail.vue'),
          meta: { requiresWorkspaceAdmin: true },
        },
        {
          path: '/training',
          name: 'Training',
          component: () => import('../pages/Training/index.vue'),
        },
        {
          path: '/torchft',
          name: 'TorchFT',
          component: () => import('../pages/TorchFT/index.vue'),
        },
        {
          path: '/rayjob',
          name: 'RayJob',
          component: () => import('../pages/RayJob/index.vue'),
        },
        {
          path: '/monarch',
          name: 'Monarch',
          component: () => import('../pages/Monarch/index.vue'),
        },
        {
          path: '/sandbox-workload',
          name: 'SandboxWorkload',
          component: () => import('../pages/SandboxWorkload/index.vue'),
        },
        {
          path: '/authoring',
          name: 'Authoring',
          component: () => import('../pages/Authoring/index.vue'),
        },
        {
          path: '/cicd',
          name: 'CICD',
          component: () => import('../pages/CICD/index.vue'),
        },
        {
          path: '/infer',
          name: 'Infer',
          component: () => import('../pages/Infer/index.vue'),
        },
        {
          path: '/cicd/detail',
          name: 'CICDDetail',
          component: () => import('../pages/CICD/CICDDetail.vue'),
        },
        {
          path: '/training/detail',
          name: 'TrainingDetail',
          component: () => import('../pages/Training/TrainingDetail.vue'),
        },
        {
          path: '/torchft/detail',
          name: 'TorchFTDetail',
          component: () => import('../pages/TorchFT/TorchFTDetail.vue'),
        },
        {
          path: '/rayjob/detail',
          name: 'RayJobDetail',
          component: () => import('../pages/RayJob/RayJobDetail.vue'),
        },
        {
          path: '/monarch/detail',
          name: 'MonarchDetail',
          component: () => import('../pages/Monarch/MonarchDetail.vue'),
        },
        {
          path: '/sandbox-workload/detail',
          name: 'SandboxWorkloadDetail',
          component: () => import('../pages/Training/TrainingDetail.vue'),
        },
        {
          path: '/training/root-cause',
          name: 'TrainingRootCause',
          component: () => import('../pages/Training/TrainingRootCauseDetail.vue'),
        },
        {
          path: '/workload/pending-cause',
          name: 'WorkloadPendingCause',
          component: () => import('../pages/Workload/PendingCauseDetail.vue'),
        },
        {
          path: '/authoring/detail',
          name: 'AuthoringDetail',
          component: () => import('../pages/Authoring/AuthoringDetail.vue'),
        },
        {
          path: '/infer/detail',
          name: 'InferDetail',
          component: () => import('../pages/Infer/InferDetail.vue'),
        },
        {
          path: '/workspace',
          name: 'Workspace',
          component: () => import('../pages/Workspace/index.vue'),
          meta: { requiresWorkspaceAdmin: true },
        },
        {
          path: '/workspace/detail',
          name: 'WorkspaceDetail',
          component: () => import('../pages/Workspace/WorkspaceDetail.vue'),
          meta: { requiresWorkspaceAdmin: true },
        },
        {
          path: '/usermanage',
          name: 'UserManage',
          component: () => import('../pages/UserManage/index.vue'),
          meta: { requiresWorkspaceAdmin: true },
        },
        {
          path: '/preflight',
          name: 'Diagnoser',
          component: () => import('../pages/Diagnoser/index.vue'),
        },
        {
          path: '/preflight/detail',
          name: 'DiagnoserDetail',
          component: () => import('../pages/Diagnoser/PreflightDetail.vue'),
          meta: { requiresWorkspaceAdmin: true },
        },
        {
          path: '/preflight/ws',
          name: 'WorkspacePreflight',
          component: () => import('../pages/Preflight/index.vue'),
        },
        {
          path: '/preflight/ws/detail',
          name: 'WorkspacePreflightDetail',
          component: () => import('../pages/Preflight/PreflightDetail.vue'),
        },
        {
          path: '/download',
          name: 'Download',
          component: () => import('../pages/Download/index.vue'),
        },
        {
          path: '/download/detail',
          name: 'DownloadDetail',
          component: () => import('../pages/Download/DownloadDetail.vue'),
        },
        {
          path: '/fault',
          name: 'Fault',
          component: () => import('../pages/Fault/index.vue'),
        },
        {
          path: '/nodeflavor',
          name: 'NodeFlavor',
          component: () => import('../pages/NodeFlavor/index.vue'),
        },
        {
          path: '/flavors/:id',
          name: 'nodeFlavorDetail',
          component: () => import('../pages/NodeFlavor/FlavorDetail.vue'),
          props: (route) => ({ id: route.params.id }),
        },
        {
          path: '/secrets',
          name: 'secrets',
          component: () => import('../pages/Secrets/index.vue'),
        },
        {
          path: '/quickstart',
          name: 'QuickStart',
          component: () => import('../pages/QuickStart/index.vue'),
        },
        {
          path: '/userquickstart',
          name: 'UserQuickStart',
          component: () => import('../pages/QuickStart/UserQuickStart.vue'),
        },
        {
          path: '/images',
          name: 'Images',
          component: () => import('../pages/Images/index.vue'),
        },
        {
          path: '/registries',
          name: 'ImageReg',
          component: () => import('../pages/ImageReg/index.vue'),
        },
        {
          path: '/webshell',
          name: 'WebShellPage',
          component: () => import('@/pages/WebShell/index.vue'),
        },
        {
          path: '/publickeys',
          name: 'PublicKeys',
          component: () => import('@/pages/PublicKeys/index.vue'),
        },
        {
          path: '/settings',
          name: 'UserSettings',
          component: () => import('@/pages/Settings/index.vue'),
        },
        {
          path: '/addontemplate',
          name: 'AddonTemp',
          component: () => import('@/pages/AddonTemp/index.vue'),
        },
        {
          path: '/addons',
          name: 'Addons',
          component: () => import('@/pages/Addons/index.vue'),
        },
        {
          path: '/posttrain',
          name: 'PostTrain',
          component: () => import('@/pages/PostTrain/index.vue'),
        },
        {
          path: '/posttrain/detail',
          name: 'PostTrainDetail',
          component: () => import('@/pages/PostTrain/PostTrainDetail.vue'),
        },
        {
          path: '/playground-agent',
          name: 'PlaygroundAgent',
          component: () => import('@/pages/PlaygroundAgent/index.vue'),
        },
        {
          path: '/model-square',
          name: 'ModelSquare',
          component: () => import('@/pages/ModelSquare/index.vue'),
        },
        {
          path: '/model-square/detail/:id',
          name: 'ModelSquareDetail',
          component: () => import('@/pages/ModelSquare/ModelSquareDetail.vue'),
        },
        {
          path: '/chatbot',
          name: 'ChatbotFullPage',
          component: () => import('@/pages/ChatbotFullPage/index.vue'),
          meta: { hideInMenu: true },
        },
        {
          path: '/qabase',
          name: 'QABase',
          component: () => import('@/pages/QABase/index.vue'),
          meta: { requiresWorkspaceAdmin: true },
        },
        {
          path: '/deploy',
          name: 'Deploy',
          component: () => import('@/pages/Deploy/index.vue'),
        },
        {
          path: '/deploy/detail',
          name: 'DeployDetail',
          component: () => import('@/pages/Deploy/DeployDetail.vue'),
        },
        {
          path: '/feedback-management',
          name: 'FeedbackManagement',
          component: () => import('@/pages/FeedbackManagement/index.vue'),
          meta: { requiresWorkspaceAdmin: true },
        },
        {
          path: '/dataset',
          name: 'Dataset',
          component: () => import('@/pages/Dataset/index.vue'),
        },
        {
          path: '/evaluation',
          name: 'Evaluation',
          component: () => import('@/pages/Evaluation/index.vue'),
        },
        {
          path: '/evaluation/:taskId',
          name: 'EvaluationDetail',
          component: () => import('@/pages/Evaluation/EvaluationDetail.vue'),
        },
        {
          path: '/model-optimization',
          name: 'ModelOptimization',
          component: () => import('@/pages/ModelOptimization/index.vue'),
        },
        {
          path: '/model-optimization/:id',
          name: 'ModelOptimizationDetail',
          component: () => import('@/pages/ModelOptimization/Detail.vue'),
          props: true,
        },
        {
          path: '/manageapikeys',
          name: 'APIKeys',
          component: () => import('@/pages/APIKeys/index.vue'),
        },
        {
          path: '/auditlogs',
          name: 'AuditLogs',
          component: () => import('@/pages/AuditLogs/index.vue'),
        },
        {
          path: '/user-group',
          name: 'UserGroup',
          component: () => import('@/pages/UserGroup/index.vue'),
        },
        {
          path: '/tools',
          name: 'Tools',
          component: () => import('@/pages/Tools/index.vue'),
        },
        {
          path: '/sandbox',
          name: 'Sandbox',
          component: () => import('@/pages/Sandbox/index.vue'),
          meta: { requiresWorkspaceAdmin: true },
        },
        {
          path: '/workload-manage',
          name: 'WorkloadManage',
          component: () => import('@/pages/WorkloadManage/index.vue'),
          meta: { requiresWorkspaceAdmin: true },
        },
        {
          path: '/litellm-gateway',
          name: 'LLMGateway',
          component: () => import('@/pages/LLMGateway/index.vue'),
        },
        {
          path: '/a2a',
          name: 'A2AProtocol',
          component: () => import('@/pages/A2A/index.vue'),
        },
        {
          path: '/claw',
          name: 'PrimusClaw',
          component: () => import('@/pages/PocoChatPage/index.vue'),
          meta: { hideInMenu: true },
        },
      ],
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'NotFound',
      component: () => import('@/pages/Error/404Page.vue'),
    },
  ],
})

setupAuthGuard(router)
setupClusterGuard(router)

// ---------------------------------------------------------------------------
// Handle dynamic-import failures (stale chunk after redeployment)
// ---------------------------------------------------------------------------
// When a new version is deployed the old hashed JS chunks are removed.
// If the user's browser still holds references to those old chunks, clicking a
// menu item triggers "Failed to fetch dynamically imported module".
// We catch that error and force a full page reload so the browser fetches the
// latest entry-point and chunk manifest.
// A sessionStorage flag prevents an infinite reload loop if the error persists.
// ---------------------------------------------------------------------------
const RELOAD_FLAG = '__dynamic_import_reload__'

router.onError((error, to) => {
  const isDynamicImportError =
    error.message?.includes('Failed to fetch dynamically imported module') ||
    error.message?.includes('Importing a module script failed') ||
    error.message?.includes('error loading dynamically imported module') ||
    error.name === 'ChunkLoadError'

  if (isDynamicImportError) {
    const reloadKey = `${RELOAD_FLAG}:${to.fullPath}`
    // Only auto-reload once per route to avoid infinite loops
    if (!sessionStorage.getItem(reloadKey)) {
      console.warn(
        `[Router] Dynamic import failed for "${to.fullPath}", reloading page to fetch latest assets…`,
      )
      sessionStorage.setItem(reloadKey, '1')
      window.location.assign(to.fullPath)
    } else {
      // Already reloaded once — clear the flag so a future deploy can retry
      sessionStorage.removeItem(reloadKey)
      console.error(
        `[Router] Dynamic import still failing after reload for "${to.fullPath}". Please hard-refresh (Ctrl+Shift+R).`,
      )
    }
  }
})

export default router
