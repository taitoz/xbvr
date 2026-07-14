const state = {
  lockScrape: false,
  lastScrapeMessage: '',
  lockRescan: false,
  lockPreview: false,
  previewGenerationStatus: '',
  previewGenerationTotal: null,
  previewGenerationLeft: null,
  lastRescanMessage: '',
  lastProgressMessage: '',
  runningScrapers: []
}

export default {
  namespaced: true,
  state
}
