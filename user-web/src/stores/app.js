import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useAppStore = defineStore('app', () => {
  const isFirstTimeAccountManagement = ref(true);
  
  const isFirstTimeMaterialManagement = ref(true);

  const isAccountRefreshing = ref(false);

  const materials = ref([]);
  const topics = ref([]);
  const categorys = ref([]);
  const setAccountManagementVisited = () => {
    isFirstTimeAccountManagement.value = false
  };
  
  const setMaterialManagementVisited = () => {
    isFirstTimeMaterialManagement.value = false
  };
  
  const resetVisitStatus = () => {
    isFirstTimeAccountManagement.value = true
    isFirstTimeMaterialManagement.value = true
  };

  const setMaterials = (materialList) => {
    materials.value = materialList
  };

  const addMaterial = (material) => {
    materials.value.push(material)
  };

  const removeMaterial = (materialId) => {
    const index = materials.value.findIndex(m => m.id === materialId)
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
    const index = categorys.value.findIndex(m => m.id === categoryId)
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
    const index = topics.value.findIndex(m => m.id === topicId)
    if (index > -1) {
      topics.value.splice(index, 1)
    }
  };
  
  const setAccountRefreshing = (status) => {
    isAccountRefreshing.value = status
  };

  return {
    isFirstTimeAccountManagement,
    isFirstTimeMaterialManagement,
    isAccountRefreshing,
    materials,
    categorys,
    topics,
    setAccountManagementVisited,
    setMaterialManagementVisited,
    resetVisitStatus,
    setMaterials,
    addMaterial,
    removeMaterial,
    setAccountRefreshing,
    setCategorys,
    addCategory,
    removeCategory,
    setTopics,
    addTopic,
    removeTopic
  }
})