import { defineStore } from 'pinia';
import { ref } from 'vue'

export const useMaterialStore = defineStore('material', () => {
  const materials = ref([]);
  const topics = ref([]);
  const categorys = ref([]);

  const setMaterials = (materialList) => {
    materials.value = materialList
  };

  const addMaterial = (material) => {
    materials.value.push(material)
  };

  const removeMaterial = (materialId) => {
    const index = materials.value.findIndex((m) => m.id === materialId)
    if (index > -1) {
      materials.value.splice(index, 1)
    }
  };

  const setCategorys = (categoryList) => {
    categorys.value = categoryList
  };

  const addCategory = (category) => {
    categorys.value.push(category)
  };

  const removeCategory = (categoryId) => {
    const index = categorys.value.findIndex((m) => m.id === categoryId)
    if (index > -1) {
      categorys.value.splice(index, 1)
    }
  };

  const setTopics = (topicList) => {
    topics.value = topicList
  };

  const addTopic = (topic) => {
    topics.value.push(topic)
  };

  const removeTopic = (topicId) => {
    const index = topics.value.findIndex((m) => m.id === topicId)
    if (index > -1) {
      topics.value.splice(index, 1)
    }
  };

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
