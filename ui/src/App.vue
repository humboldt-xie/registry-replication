<template>
  <div id="app">
    <el-progress :percentage="progress"></el-progress>
    <el-tabs v-model="activeName" tab-click="handleClick">
    <el-tab-pane v-for="(status,key) in statusData" v-bind:key="key" :label="key" :name="key">{{key}}</el-tab-pane>
    </el-tabs>
     <el-table
      :data="tableData"
      style="width: 100%">
      <el-table-column
        prop="name"
        label="项目"
        width="180">
      </el-table-column>
      <el-table-column
        prop="status"
        label="status">
      </el-table-column>
      <el-table-column
        label="tags">
        <template slot-scope="scope" >
          <li v-for="tag in scope.row.tags" v-bind:key="tag.name" >
            {{tag.name}} - {{tag.status}}
          </li>
        </template>
      </el-table-column>
    </el-table>
    <!--router-view/-->
  </div>
</template>

<script>
import store from './store'
export default {
  name: 'App',
  vuex: {
  },
  computed: {
      statusData: function(){
        return store.state.statusData
      },
      tableData: function(){
        return store.state.statusData[this.activeName]
      },
      progress: function(){
        return store.state.progress
      }
  },
  data() {
    return {
      activeName: 'test'
    };
  },
  created () {
    this.initData()
  }/* ,
  computed: {
    progress () {
      return this.$store.state.progress
    }
  } */
}
</script>

<style>
#app {
  font-family: 'Avenir', Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  text-align: center;
  color: #2c3e50;
  margin-top: 60px;
}
</style>
