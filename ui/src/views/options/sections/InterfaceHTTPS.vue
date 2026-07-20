<template>
  <div class="content">
    <b-loading :is-full-page="true" :active.sync="isLoading"></b-loading>
    <h3 class="title">{{ $t('HTTPS / DuckDNS') }}</h3>
    <p class="subtitle" style="margin-bottom: 1.5rem;">
      {{ $t('Enable HTTPS access via Caddy reverse proxy with automatic DuckDNS IP updates.') }}
    </p>

    <b-field :label="$t('DuckDNS Domain')">
      <b-input v-model="duckDomain" :placeholder="'myxbvr'" expanded></b-input>
    </b-field>
    <p class="help" style="margin-top: -0.75rem; margin-bottom: 1rem;">
      {{ $t('Enter only the subdomain part (e.g. myxbvr for myxbvr.duckdns.org)') }}
    </p>

    <b-field :label="$t('DuckDNS Token')">
      <b-input v-model="duckToken" type="password" :placeholder="'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx'" expanded></b-input>
    </b-field>

    <b-field>
      <b-switch v-model="enabled">
        {{ $t('Enable HTTPS') }}
      </b-switch>
    </b-field>

    <b-field>
      <b-switch v-model="autoStart">
        {{ $t('Autostart with XBVR') }}
      </b-switch>
    </b-field>

    <div style="margin-top: 1.5rem;">
      <b-button type="is-primary" @click="save" :loading="isSaving">{{ $t('Save') }}</b-button>
    </div>

    <div v-if="caddyRunning" style="margin-top: 1rem;">
      <b-tag type="is-success">{{ $t('Caddy is running') }}</b-tag>
      <span style="margin-left: 0.5rem;" v-if="duckDomain">
        {{ $t('HTTPS available at') }} <a :href="'https://' + duckDomain + '.duckdns.org'" target="_blank">https://{{ duckDomain }}.duckdns.org</a>
      </span>
    </div>
    <div v-else style="margin-top: 1rem;">
      <b-tag type="is-light">{{ $t('Caddy is not running') }}</b-tag>
    </div>
  </div>
</template>

<script>
import ky from 'ky'

export default {
  name: 'InterfaceHTTPS',
  data () {
    return {
      isLoading: true,
      isSaving: false,
      enabled: false,
      duckDomain: '',
      duckToken: '',
      autoStart: false,
      caddyRunning: false
    }
  },
  async mounted () {
    await this.loadState()
  },
  methods: {
    async loadState () {
      this.isLoading = true
      try {
        const data = await ky.get('/api/options/state').json()
        this.enabled = data.config.https.enabled
        this.duckDomain = data.config.https.duckDomain || ''
        this.duckToken = data.config.https.duckToken || ''
        this.autoStart = data.config.https.autoStart
        this.caddyRunning = this.enabled
      } catch (e) {
        // ignore
      }
      this.isLoading = false
    },
    async save () {
      this.isSaving = true
      try {
        const data = await ky.put('/api/options/https', {
          json: {
            enabled: this.enabled,
            duckDomain: this.duckDomain,
            duckToken: this.duckToken,
            autoStart: this.autoStart
          }
        }).json()
        this.caddyRunning = data.enabled
        this.$buefy.toast.open({
          message: this.$t('Settings saved'),
          type: 'is-success'
        })
      } catch (e) {
        this.$buefy.toast.open({
          message: this.$t('Failed to save settings'),
          type: 'is-danger'
        })
      }
      this.isSaving = false
    }
  }
}
</script>
