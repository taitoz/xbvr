<template>
  <b-navbar :fixed-top="true" type="is-light">
    <template slot="brand">
      <a v-if="showFiltersBurger" role="button" class="navbar-burger burger filters-burger" @click="$store.commit('overlay/toggleFilters')" :class="{'is-active': filtersOpen}" :title="filtersOpen ? 'Hide filters' : 'Show filters'" aria-label="filters menu">
        <span aria-hidden="true"></span>
        <span aria-hidden="true"></span>
        <span aria-hidden="true"></span>
      </a>
      <b-navbar-item>
        <h1 class="title"><img class="brand-logo" :src="logoSrc" alt="XBVR"/> <small class="version-tag">{{displayVersion}}</small></h1>
      </b-navbar-item>
    </template>
    <template slot="start">
      <b-navbar-item tag="router-link" :to="{ path: './' }">
        {{$t('Scenes')}}
      </b-navbar-item>
      <b-navbar-item tag="router-link" :to="{ path: './actors' }">
        {{$t('Actors')}}
      </b-navbar-item>
      <b-navbar-item tag="router-link" :to="{ path: './files' }">
        {{$t('Files')}}
      </b-navbar-item>
      <b-navbar-item tag="router-link" :to="{ path: './options' }">
        {{$t('Options')}}
      </b-navbar-item>
      <b-navbar-item class="quick-find">
        <b-autocomplete
          ref="autocompleteInput"
          :data="data"
          placeholder="I'm looking for..."
          field="query"
          :loading="isFetching"
          v-model="queryString"
          @typing="getAsyncData"
          @select="showSceneDetails"
          :open-on-focus="true"
          :clearable="true"
          max-height="450">
          <template slot-scope="props">
            <div class="media" v-if="props.option._type === 'actor'">
              <div class="media-left">
                <vue-load-image>
                  <img slot="image" :src="getActorImageURL(props.option.image_url)" width="64"/>
                  <img slot="preloader" src="/ui/images/blank.png" width="64"/>
                  <img slot="error" src="/ui/images/blank_female_profile.png" width="64"/>
                </vue-load-image>
              </div>
              <div class="media-content">
                <div class="truncate"><strong>{{ props.option.name }}</strong></div>
                <small v-if="props.option.aliases">{{ props.option.aliases }}</small><br/>
                <small>{{ props.option.avail_count }} scenes</small>
              </div>
            </div>
            <div class="media" v-else>
              <div class="media-left">
                <vue-load-image>
                  <img slot="image" :src="getImageURL(props.option.cover_url)" width="64"/>
                  <img slot="preloader" src="/ui/images/blank.png" width="64"/>
                  <img slot="error" src="/ui/images/blank.png" width="64"/>
                </vue-load-image>
              </div>
              <div class="media-content">
                {{ props.option.site }}
                <b-icon v-if="props.option.is_hidden" pack="mdi" icon="eye-off-outline" size="is-small"/><br/>
                <div class="truncate"><strong>{{ props.option.title }}</strong></div>
                <small>
                  <span v-for="(c, idx) in props.option.cast" :key="'cast' + idx">
                    {{ c.name }}<span v-if="idx < props.option.cast.length - 1">, </span>
                  </span>
                </small>
                <star-rating v-if="props.option.star_rating != 0" :read-only="true" :rating="props.option.star_rating" :increment="0.5" :show-rating="false" :star-size="10"/>
              </div>
              <div class="media-right">
                {{ format(parseISO(props.option.release_date), 'yyyy-MM-dd') }}
              </div>
            </div>
          </template>
        </b-autocomplete>
      </b-navbar-item>
    </template>
    <template slot="end">
      <b-navbar-item>
        <table style="font-size:0.9em">
          <tr v-if="Object.keys(lastRescanMessage).length !== 0 || previewGenerationStatus">
            <th><span :class="[(lockRescan || lockPreview) ? 'pulsate' : '']">{{$t('Files')}} →</span></th>
            <td>
              <span v-if="Object.keys(lastRescanMessage).length !== 0">{{lastRescanMessage.message}}</span>
              <span v-if="previewGenerationStatus">
                <span v-if="Object.keys(lastRescanMessage).length !== 0"> | </span>
                Preview generation<span v-if="lockPreview"> Total: {{ previewGenerationTotal }} Left: {{ previewGenerationLeft }}</span><span v-else> complete</span>
              </span>
            </td>
          </tr>
          <tr v-if="Object.keys(lastScrapeMessage).length !== 0">
            <th><span :class="[lockScrape ? 'pulsate' : '']">{{$t('Data')}} →</span></th>
            <td>{{lastScrapeMessage.message}}</td>
          </tr>
        </table>
      </b-navbar-item>
    </template>
  </b-navbar>
</template>

<script>
import ky from 'ky'
import VueLoadImage from 'vue-load-image'
import { format, parseISO } from 'date-fns'
import StarRating from 'vue-star-rating'

export default {
  components: { VueLoadImage, StarRating },
  data () {
    return {
      currentVersion: '',
      latestVersion: '',
      data: [],
      dataNumRequests: 0,
      dataNumResponses: 0,
      isFetching: false,
      queryString: '',
      previewProgressInterval: null
    }
  },
  computed: {
    showFiltersBurger () {
      return this.$route && (this.$route.path === '/' || this.$route.path === '/actors')
    },
    filtersOpen () {
      return this.$store.state.overlay.filtersOpen
    },
    lockRescan () {
      return this.$store.state.messages.lockRescan
    },
    lastRescanMessage () {
      return this.$store.state.messages.lastRescanMessage
    },
    lockScrape () {
      return this.$store.state.messages.lockScrape
    },
    lockPreview () {
      return this.$store.state.messages.lockPreview
    },
    previewGenerationStatus () {
      return this.$store.state.messages.previewGenerationStatus
    },
    previewGenerationTotal () {
      return this.$store.state.messages.previewGenerationTotal
    },
    previewGenerationLeft () {
      return this.$store.state.messages.previewGenerationLeft
    },
    lastScrapeMessage () {
      return this.$store.state.messages.lastScrapeMessage
    },
    displayVersion () {
      return this.currentVersion === 'CURRENT' ? 'dev build' : this.currentVersion
    },
    quickFindVisible () {
      return this.$store.state.overlay.quickFind.show
    },
    logoSrc () {
      return this.$store.state.optionsWeb.web.theme === 'dark'
        ? '/ui/images/xbvr-logo.png'
        : '/ui/images/xbvr-logo-dark.png'
    }
  },
  watch: {
    quickFindVisible (show) {
      if (!show) {
        return
      }
      this.$nextTick(() => {
        const searchString = this.$store.state.overlay.quickFind.searchString
        if (searchString) {
          this.queryString = searchString
          this.$store.state.overlay.quickFind.searchString = null
          this.getAsyncData(searchString)
        }
        this.$refs.autocompleteInput.$refs.input.focus()
      })
    },
    lockPreview (locked) {
      if (locked) {
        this.fetchPreviewProgress()
        this.previewProgressInterval = setInterval(this.fetchPreviewProgress, 3000)
      } else if (this.previewProgressInterval) {
        clearInterval(this.previewProgressInterval)
        this.previewProgressInterval = null
      }
    }
  },
  mounted () {
    ky.get('/api/options/version-check').json().then(data => {
      this.currentVersion = data.current_version
      this.latestVersion = data.latest_version

      if (data.update_notify && this.currentVersion !== 'CURRENT') {
        this.$buefy.snackbar.open({
          message: `Version ${this.latestVersion} available!`,
          type: 'is-warning',
          position: 'is-bottom-right',
          actionText: this.$t('Download now'),
          indefinite: true,
          onAction: () => {
            window.location = 'https://github.com/xbapps/xbvr/releases'
          }
        })
      }
    })
  },
  beforeDestroy () {
    if (this.previewProgressInterval) {
      clearInterval(this.previewProgressInterval)
    }
  },
  methods: {
    format,
    parseISO,
    async getAsyncData (query) {
      const requestIndex = this.dataNumRequests
      this.dataNumRequests += 1
      if (!query.length) {
        this.data = []
        this.dataNumResponses = requestIndex + 1
        this.isFetching = false
        return
      }
      this.isFetching = true
      const [sceneResp, actorResp] = await Promise.all([
        ky.get('/api/scene/search', { searchParams: { q: query } }).json(),
        ky.get('/api/actor/search', { searchParams: { q: query } }).json()
      ])
      if (requestIndex >= this.dataNumResponses) {
        this.dataNumResponses = requestIndex + 1
        if (this.dataNumResponses === this.dataNumRequests) {
          this.isFetching = false
        }
        const scenes = (sceneResp.results > 0 ? sceneResp.scenes : []).map(s => { s._type = 'scene'; return s })
        const actors = (actorResp.results > 0 ? actorResp.actors : []).map(a => { a._type = 'actor'; return a })
        this.data = [...actors, ...scenes]
      }
    },
    async fetchPreviewProgress () {
      try {
        const data = await ky.get('/api/task/preview/count').json()
        const messages = this.$store.state.messages
        if (messages.previewGenerationTotal === null) {
          messages.previewGenerationTotal = data.left
        }
        messages.previewGenerationLeft = data.left
      } catch (e) {
        // ignore
      }
    },
    getImageURL (u) {
      if (u && u.startsWith('http')) {
        return '/img/120x/' + u.replace('://', ':/')
      }
      return u || '/ui/images/blank.png'
    },
    showSceneDetails (item) {
      if (!item) {
        return
      }
      this.data = []
      this.queryString = ''
      if (item._type === 'actor') {
        this.$store.commit('overlay/showActorDetails', { actor: item })
        this.$store.commit('overlay/hideQuickFind')
        return
      }
      const quickFind = this.$store.state.overlay.quickFind
      if (quickFind.displaySelectedScene) {
        if (this.$router.currentRoute.name !== 'scenes') {
          this.$router.push({ name: 'scenes' })
        }
        this.$store.commit('overlay/showDetails', { scene: item })
      } else {
        quickFind.selectedScene = item
      }
      this.$store.commit('overlay/hideQuickFind')
    },
    getActorImageURL (u) {
      if (u && u.startsWith('http')) {
        return '/img/120x/' + u.replace('://', ':/')
      }
      return u || '/ui/images/blank_female_profile.png'
    }
  }
}
</script>

<style scoped>
  /* Force filters burger visible at all screen sizes, left-aligned */
  .filters-burger {
    display: flex !important;
    margin-left: 0 !important;
    margin-right: 0 !important;
    order: -1;
  }

  h1 {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    line-height: 1;
  }

  .brand-logo {
    height: 34px;
    width: auto;
  }

  .version-tag {
    display: block;
    font-size: 11px;
    opacity: 0.6;
    margin-left: 0;
    margin-top: 2px;
  }

  .quick-find {
    position: absolute;
    left: 51%;
    width: 700px;
    padding: 0;
    transform: translateX(-50%);
  }

  .quick-find ::v-deep .autocomplete,
  .quick-find ::v-deep .control {
    width: 100%;
  }

  .quick-find ::v-deep .dropdown-menu {
    z-index: 40;
  }

  @media screen and (max-width: 1023px) {
    .quick-find {
      position: static;
      width: 100%;
      padding: 0.5rem 1rem;
      transform: none;
    }
  }

  th {
    padding-right: 1em;
  }

  .pulsate {
    -webkit-animation: pulsate 0.5s linear;
    -webkit-animation-iteration-count: infinite;
    opacity: 0.5;
  }

  @-webkit-keyframes pulsate {
    0% {
      opacity: 0.5;
    }
    50% {
      opacity: 1.0;
    }
    100% {
      opacity: 0.5;
    }
  }
</style>
