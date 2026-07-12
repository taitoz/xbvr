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
    <List/>

    <a id="toTop">
      <b-icon pack="mdi" icon="navigation" />
    </a>

  </div>
</template>

<script>
import Filters from './Filters'
import List from './List'

export default {
  name: 'Actors',
  components: { Filters, List },
  computed: {
    filtersOpen() {
      return this.$store.state.overlay.filtersOpen
    }
  },
  methods: {
    closeFilters() {
      this.$store.commit('overlay/closeFilters')
    }
  },
  mounted () {
    const toTop = document.getElementById('toTop')
    addEventListener('scroll', function () {
      toTop.style.display = document.body.scrollTop > 20 || document.documentElement.scrollTop > 20
        ? 'block'
        : 'none'
    })
    toTop.addEventListener('click', function () {
      window.scrollTo({ top: 0, behavior: 'smooth' })
    })
  },
  beforeRouteEnter (to, from, next) {
    next(vm => {
      if (to.query !== undefined) {
        vm.$store.commit('actorList/stateFromQuery', to.query)
      }
      vm.$store.dispatch('optionsWeb/load')
      const page = parseInt(to.query.page) || 1
      const limit = vm.$store.state.actorList.limit
      const offset = (page - 1) * limit
      vm.$store.dispatch('actorList/load', { offset })
      vm.$store.dispatch('optionsAdvanced/load')
    })
  },
  beforeRouteUpdate (to, from, next) {
    if (to.query !== undefined) {
      this.$store.commit('actorList/stateFromQuery', to.query)
    }
    const page = parseInt(to.query.page) || 1
    const limit = this.$store.state.actorList.limit
    const offset = (page - 1) * limit
    this.$store.dispatch('actorList/load', { offset })
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
  #toTop {
    display: none;
    position: fixed;
    bottom: 20px;
    left: 30px;
    background-color: #f0f0f0;
    color: #4a4a4a;
    padding: 15px;
    border-radius: 10px;
    font-size: 18px;
    z-index: 1000;
  }
  #toTop:hover {
    background-color: #BDBDBD;
  }
</style>
