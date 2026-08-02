// 素材业务数据 Store
//
// 从原 useAppStore 拆分出来的素材 / 话题 / 分类业务数据。
//   - materials：素材列表（MaterialSelectDialog 使用）
//   - topics：话题库列表
//   - categorys：分类列表
//
// 使用方式：
//   import { useMaterialStore } from '@/stores/material'
//   const mat = useMaterialStore()
//   mat.setMaterials(list)
import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useMaterialStore = defineStore('material', () => {
  // 素材列表数据
  const materials = ref([])
  // 话题库列表数据
  const topics = ref([])
  // 分类列表数据
  const categorys = ref([])

  // 更新素材列表
  const setMaterials = (materialList) => {
    materials.value = materialList
  }

  // 添加新素材
  const addMaterial = (material) => {
    materials.value.push(material)
  }

  // 删除素材
  const removeMaterial = (materialId) => {
    const index = materials.value.findIndex((m) => m.id === materialId)
    if (index > -1) {
      materials.value.splice(index, 1)
    }
  }

  // 更新分类列表
  const setCategorys = (categoryList) => {
    categorys.value = categoryList
  }

  // 添加新分类
  const addCategory = (category) => {
    categorys.value.push(category)
  }

  // 删除分类
  const removeCategory = (categoryId) => {
    const index = categorys.value.findIndex((m) => m.id === categoryId)
    if (index > -1) {
      categorys.value.splice(index, 1)
    }
  }

  // 更新话题库列表
  const setTopics = (topicList) => {
    topics.value = topicList
  }

  // 添加新话题
  const addTopic = (topic) => {
    topics.value.push(topic)
  }

  // 删除话题
  const removeTopic = (topicId) => {
    const index = topics.value.findIndex((m) => m.id === topicId)
    if (index > -1) {
      topics.value.splice(index, 1)
    }
  }

  return {
    materials,
    topics,
    categorys,
    setMaterials,
    addMaterial,
    removeMaterial,
    setCategorys,
    addCategory,
    removeCategory,
    setTopics,
    addTopic,
    removeTopic
  }
})

export default useMaterialStore
