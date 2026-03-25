// Menu icon configuration
// This file centralizes all menu icon imports for better maintainability

export interface IconSet {
  light: string
  dark: string
  active: string
}

export interface MenuIcons {
  queue: IconSet
  node: IconSet
  cluster: IconSet
  training: IconSet
  torchft: IconSet
  authoring: IconSet
  dataset: IconSet
  evaluation: IconSet
  usermanage: IconSet
  fault: IconSet
  flavors: IconSet
  secrets: IconSet
  diagnoser: IconSet
  registry: IconSet
  addon: IconSet
  addons: IconSet
  images: IconSet
  quickstart: IconSet
  cicd: IconSet
  deploy: IconSet
  download: IconSet
  infer: IconSet
  modelSquare: IconSet
  preflight: IconSet
  qabase: IconSet
  playground: IconSet
  apikey: IconSet
  chatbot: IconSet
  rayjob: IconSet
  tools: IconSet
  llmGateway: IconSet
  a2a: IconSet
}

const createIconSet = (name: string): IconSet => ({
  light: new URL(`../../assets/icons/${name}-light.png`, import.meta.url).href,
  dark: new URL(`../../assets/icons/${name}.png`, import.meta.url).href,
  active: new URL(`../../assets/icons/${name}-active.png`, import.meta.url).href,
})

export const menuIcons: MenuIcons = {
  queue: createIconSet('queue'),
  node: createIconSet('nodes'),
  cluster: createIconSet('cluster'),
  training: createIconSet('training'),
  torchft: createIconSet('torchft'),
  authoring: createIconSet('authoring'),
  usermanage: createIconSet('usermanage'),
  fault: createIconSet('fault'),
  flavors: createIconSet('flavors'),
  secrets: createIconSet('secrets'),
  diagnoser: createIconSet('diagnoser'),
  registry: createIconSet('registry'),
  addon: createIconSet('addon'),
  addons: createIconSet('addons'),
  images: createIconSet('images'),
  quickstart: createIconSet('quickstart'),
  cicd: createIconSet('cicd'),
  deploy: createIconSet('deploy'),
  download: createIconSet('deploy'), // Using deploy icons as placeholder
  infer: createIconSet('infer'),
  dataset: createIconSet('dataset'),
  evaluation: createIconSet('evaluation'),
  modelSquare: createIconSet('model'),
  preflight: createIconSet('diagnoser'),
  qabase: createIconSet('knowledge'),
  playground: createIconSet('infer'), // Using infer icons as placeholder
  apikey: createIconSet('apikey'),
  chatbot: createIconSet('sparkles'),
  rayjob: createIconSet('ray'),
  tools: createIconSet('skill'), // Using addons icons as placeholder
  llmGateway: createIconSet('apikey'), // Using apikey icons as placeholder
  a2a: createIconSet('skill'), // Using skill icons as placeholder
}
