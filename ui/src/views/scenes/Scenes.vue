<template>
  <div class="container is-fluid">

    <!-- Filters sidebar overlay -->
    <transition name="slide">
      <div v-if="filtersOpen" class="filters-sidebar">
        <Filters/>
      </div>
    </transition>
    <div v-if="filtersOpen" class="filters-backdrop" @click="closeFilters"/>

    <!-- Full-width list -->
    <List :infinite-scroll-enabled="infiniteScrollEnabled"/>

    <div id="scrollButtons">
      <a id="toTop">
        <b-icon pack="mdi" icon="navigation" />
      </a>
      <a id="toggleInfiniteScroll" @click="toggleInfiniteScroll" :title="infiniteScrollEnabled ? 'Disable Auto Load More' : 'Enable Auto Load More'">
        <b-icon pack="mdi" :icon="infiniteScrollEnabled ? 'reload' : 'pause'" />
      </a>
    </div>

  </div>
</template>

<script>
import Filters from './Filters'
import List from './List'

export default {
  name: 'Scenes',
  components: { Filters, List },
  data() {
    return {
      infiniteScrollEnabled: true
    }
  },
  computed: {
    filtersOpen() {
      return this.$store.state.overlay.filtersOpen
    }
  },
  methods: {
    toggleInfiniteScroll() {
      this.infiniteScrollEnabled = !this.infiniteScrollEnabled
    },
    closeFilters() {
      this.$store.commit('overlay/closeFilters')
    }
  },
  mounted () {
    const toTop = document.getElementById('toTop')
    const toggleBtn = document.getElementById('toggleInfiniteScroll')
    addEventListener('scroll', function () {
      const show = document.body.scrollTop > 20 || document.documentElement.scrollTop > 20
      toTop.style.display = show ? 'block' : 'none'
      toggleBtn.style.display = show ? 'block' : 'none'
    })
    toTop.addEventListener('click', function () {
      window.scrollTo({ top: 0, behavior: 'smooth' })
    })
  },
  beforeRouteEnter (to, from, next) {
    next(vm => {
      if (to.query !== undefined) {
        vm.$store.commit('sceneList/stateFromQuery', to.query)
      }
      vm.$store.dispatch('optionsWeb/load')
      vm.$store.dispatch('sceneList/load', { offset: 0 })
      vm.$store.dispatch('optionsAdvanced/load')
    })
  },
  beforeRouteUpdate (to, from, next) {
    if (to.query !== undefined) {
      this.$store.commit('sceneList/stateFromQuery', to.query)
    }
    this.$store.dispatch('sceneList/load', { offset: 0 })
    next()
  },
}
</script>

<style scoped>
  .filters-sidebar {
    position: fixed;
    top: 52px;
    left: 0;
    width: 300px;
    height: calc(100vh - 52px);
    z-index: 998;
    overflow-y: auto;
    padding: 1rem;
    box-shadow: 2px 0 8px rgba(0,0,0,0.15);
  }
  .filters-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.4);
    z-index: 997;
  }
  .slide-enter-active, .slide-leave-active {
    transition: transform 0.25s ease;
  }
  .slide-enter, .slide-leave-to {
    transform: translateX(-100%);
  }
  #scrollButtons {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    position: fixed;
    bottom: 20px;
    right: 30px;
    z-index: 1000;
  }
  #toTop, #toggleInfiniteScroll {
    display: none;
    background-color: #f0f0f0;
    color: #4a4a4a;
    padding: 15px;
    border-radius: 10px;
    font-size: 18px;
    box-shadow: 0 2px 5px rgba(0, 0, 0, 0.3);
    cursor: pointer;
  }
  #toTop:hover, #toggleInfiniteScroll:hover {
    background-color: #BDBDBD;
  }
</style>
