<template>
  <b-navbar :fixed-top="true" type="is-light">
    <template slot="brand">
      <a v-if="showFiltersBurger" role="button" class="navbar-burger burger filters-burger" @click="$store.commit('overlay/toggleFilters')" :class="{'is-active': filtersOpen}" :title="filtersOpen ? 'Hide filters' : 'Show filters'" aria-label="filters menu">
        <span aria-hidden="true"></span>
        <span aria-hidden="true"></span>
        <span aria-hidden="true"></span>
      </a>
      <b-navbar-item>
        <h1 class="title">XBVR <small class="version-tag">{{displayVersion}}</small></h1>
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
      <b-navbar-item @click="$store.commit('overlay/showQuickFind')">
        {{$t('Quick find')}}
      </b-navbar-item>
    </template>
    <template slot="end">
      <b-navbar-item>
        <table style="font-size:0.9em">
          <tr v-if="Object.keys(lastRescanMessage).length !== 0">
            <th><span :class="[lockRescan ? 'pulsate' : '']">{{$t('Files')}} →</span></th>
            <td>{{lastRescanMessage.message}}</td>
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

export default {
  data () {
    return {
      currentVersion: '',
      latestVersion: ''
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
    lastScrapeMessage () {
      return this.$store.state.messages.lastScrapeMessage
    },
    displayVersion () {
      return this.currentVersion === 'CURRENT' ? 'dev build' : this.currentVersion
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

  .version-tag {
    display: block;
    font-size: 11px;
    opacity: 0.6;
    margin-left: 0;
    margin-top: 2px;
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
