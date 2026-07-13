<template>
  <div>
    <GlobalEvents
      :filter="e => !['INPUT', 'TEXTAREA'].includes(e.target.tagName)"
      @keydown.left="prevpage"
      @keydown.right="nextpage"
      @keydown.o="prevpage"
      @keydown.p="nextpage"
    />
    <b-loading :is-full-page="true" :active.sync="isLoading"></b-loading>

    <div class="columns is-multiline is-full is-centered">
      <div class="column has-text-centered">
        <strong>{{total}} results</strong>
      </div>
      <div class="column is-narrow has-text-centered">
        <div class="columns is-gapless is-centered">
          <b-radio-button v-model="availFilter" native-value="any" size="is-small">
            {{ $t('Any') }} ({{countAny}})
          </b-radio-button>
          <b-radio-button v-model="availFilter" native-value="available" size="is-small">
            {{ $t('Available') }} ({{countAvailable}})
          </b-radio-button>
        </div>
      </div>
      <div class="column has-text-centered">
        <b-tooltip :label="$t('Press o/left arrow to page back, p/right arrow to page forward')" :delay="500" position="is-top">
          <b-pagination
              :total="total"
              v-model="current"
              range-before=1
              range-after=3    
              size="is-small"                                           
              :per-page="limit"
              aria-next-label="Next page"
              aria-previous-label="Previous page"
              aria-page-label="Page"
              aria-current-label="Current page"
              :page-input=true
              @change="pageChanged"
              debounce-page-input="250"
              >
          </b-pagination>
        </b-tooltip>
        <span v-show="show_actor_id==='never show, just need the computed show_actor_id to trigger '">{{show_actor_id}}</span>
      </div>
    </div>
        <div class="columns is-gapless is-centered" v-if="hideLetters">
          <b-radio-button v-model="jumpTo" native-value="" size="is-small"></b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="A" size="is-small">A</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="B" size="is-small">B</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="C" size="is-small">C</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="D" size="is-small">D</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="E" size="is-small">E</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="F" size="is-small">F</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="G" size="is-small">G</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="H" size="is-small">H</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="I" size="is-small">I</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="J" size="is-small">J</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="K" size="is-small">K</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="L" size="is-small">L</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="M" size="is-small">M</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="N" size="is-small">N</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="O" size="is-small">O</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="P" size="is-small">P</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="Q" size="is-small">Q/R</b-radio-button>          
          <b-radio-button v-model="jumpTo" native-value="S" size="is-small">S</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="T" size="is-small">T</b-radio-button>
          <b-radio-button v-model="jumpTo" native-value="U" size="is-small">U/V</b-radio-button>          
          <b-radio-button v-model="jumpTo" native-value="W" size="is-small">W/X/Y/Z</b-radio-button>
        </div>

    <div class="is-clearfix"></div>

    <div class="grid-actors">
      <ActorCard v-for="actor in actors" :key="actor.id" :actor="actor"/>
    </div>
  </div>
</template>

<script>
import ActorCard from './ActorCard'
import ky from 'ky'
import GlobalEvents from 'vue-global-events'

export default {
  name: 'List',
  components: { ActorCard, GlobalEvents },
  data () {
    return {
      current: 1,
      windowWidth: window.innerWidth,
      resizeHandler: null
    }
  },
  created () {
    const page = parseInt(this.$route.query.page) || 1
    this.current = page
    this.updateLimit(this.calculatedLimit, false)
  },
  computed: {
    columnsPerRow () {
      // 170px covers 150px card + gap/margins
      return Math.max(1, Math.floor(this.windowWidth / 170))
    },
    calculatedLimit () {
      // exactly 3 rows per page
      return this.columnsPerRow * 3
    },
    limit () {
      return this.$store.state.actorList.limit
    },
    jumpTo: {
      get () {
        return this.$store.state.actorList.filters.jumpTo
      },
      set (value) {
        this.$store.state.actorList.filters.jumpTo = value
        this.current = 1
        this.reloadList()
      }
    },
    isLoading () {
      this.current = this.$store.state.actorList.offset / this.$store.state.actorList.limit
      return this.$store.state.actorList.isLoading
    },
    actors () {
      return this.$store.state.actorList.actors
    },
    total () {
      return this.$store.state.actorList.total
    },
    show_actor_id() {
      if (this.$store.state.actorList.show_actor_id != undefined && this.$store.state.actorList.show_actor_id !='')
      {
        ky.get('/api/actor/'+this.$store.state.actorList.show_actor_id).json().then(data => {
          if (data.id != 0){
            this.$store.commit('overlay/showActorDetails', { actor: data })
          }          
        })
        this.$store.state.actorList.show_actor_id = ''
      }
      
      return this.$store.state.actorList.show_actor_id
    },
    countAny () {
      return this.$store.state.actorList.countAny
    },
    countAvailable () {
      return this.$store.state.actorList.countAvailable
    },
    availFilter: {
      get () {
        return this.$store.state.actorList.filters.min_avail > 0 ? 'available' : 'any'
      },
      set (value) {
        this.$store.state.actorList.filters.min_avail = value === 'available' ? 1 : 0
        this.current = 1
        this.reloadList()
      }
    },
    hideLetters: {
      get () {        
        switch (this.$store.state.actorList.filters.sort) {
          case "":
            return true
          case "name_asc":
            return true
          case "name_desc":
            return true
        }
        return false
        },
    },
  },
  watch: {
    windowWidth () {
      this.updateLimit(this.calculatedLimit, true)
    }
  },
  methods: {
    updateLimit (newLimit, reload) {
      if (newLimit === this.$store.state.actorList.limit) return
      // find the position of the first actor
      let currentOffset = this.$store.state.actorList.offset - this.$store.state.actorList.limit + 1
      // what is the new page number, based on the new limit
      this.current = Math.floor(currentOffset / newLimit) + 1
      if (this.current < 1) this.current = 1
      this.$store.state.actorList.limit = newLimit
      // what is the first actor based on the new page size
      this.$store.state.actorList.offset = (this.current - 1) * this.$store.state.actorList.limit
      if (reload) {
        this.$store.dispatch('actorList/load', { offset: this.$store.state.actorList.offset })
      }
    },
    reloadList () {
      this.$router.push({
        name: 'actors',
        query: {
          q: this.$store.getters['actorList/filterQueryParams'],
          page: this.current
        }
      })
    },
    async pageChanged () {
      this.$store.state.actorList.offset = (this.current - 1) * this.$store.state.actorList.limit
      this.$router.push({
        name: 'actors',
        query: {
          ...this.$route.query,
          page: this.current
        }
      }).catch(() => {})
      this.$store.dispatch('actorList/load', { offset: this.$store.state.actorList.offset })
    },
    nextpage () {
      if (this.$store.state.overlay.actordetails.show){
        return 
      }
      if (this.$store.state.overlay.details.show){
        return 
      }
      if (this.current * this.limit >= this.total) {
        this.current = 1
      } else {
        this.current += 1
      }      
      this.pageChanged()
    },
    prevpage () {      
      if (this.$store.state.overlay.actordetails.show){
        return 
      }
      if (this.$store.state.overlay.details.show){
        return 
      }
      if (this.current > 1) {
        this.current -= 1
      } else {
        this.current = Math.floor(this.total / this.limit) + 1        
      }      
      this.pageChanged()
    },
  },
  mounted () {
    this.resizeHandler = () => { this.windowWidth = window.innerWidth }
    window.addEventListener('resize', this.resizeHandler)
  },
  beforeDestroy () {
    if (this.resizeHandler) {
      window.removeEventListener('resize', this.resizeHandler)
    }
  }
}
</script>

<style scoped>
  .list-header-label {
    padding-right: 1em;
  }
  .grid-actors {
    display: grid;
    grid-template-columns: repeat(auto-fill, 150px);
    gap: 1rem;
    justify-content: center;
  }
</style>
