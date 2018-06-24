/**
 * Vuex
 * http://vuex.vuejs.org/zh-cn/intro.html
 */
import Vue from 'vue'
import Vuex from 'vuex'
import axios from 'axios'
import jsync from './jsync'

Vue.use(Vuex)
function getProgress (data) {
  var i
  var total = 0
  var done = 0
  for (i in data) {
    var reposition = data[i]
    if (reposition.Status === 'done') {
      done++
    }
    total++
  }
  if (total === 0) {
    return 0
  }
  return done / total * 100
}

// const now = new Date()
const store = new Vuex.Store({
  state: {
    progress: 25,
    statusData: [
      {
        name: 'busybox',
        tags: [
          {tag: 'v1', status: 'ok'},
          {tag: 'v1', status: 'ok'},
          {tag: 'v1', status: 'ok'}
        ]
      }
    ]
  },
  mutations: {
    get (state) {
      jsync.jsync({
        'status': function (name,resp) {
          state.statusData = resp
        }
      })
      // 变更状态
      // state.count++
      axios.get('/status/test').then(function (res) {
        state.progress = getProgress(res.data)
        //state.state.progress = 30
        console.log(res, state.progress)
        // this.list = res.data.data.list
        // ref  list 引用了ul元素，我想把第一个li颜色变为红色
        // this.$refs.list.getElementsByTagName('li')[0].style.color = 'red'
      })
    }
  }
})

export default store
