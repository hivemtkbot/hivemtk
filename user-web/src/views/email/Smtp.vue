<template>
  <div class="email-settings">
    <div class="toolbar">
      <el-button type="primary" @click="handleAdd">
        <el-icon-plus /> {{ $t('新增邮件代理') }}
      </el-button>
    </div>

    <!-- 邮件代理列表 -->
    <el-table :data="tableData" border stripe style="width: 100%">
      <el-table-column prop="name" :label="$t('代理名称')" width="300" />
      <el-table-column prop="server" :label="$t('服务器地址')" width="200" />
      <el-table-column prop="port" :label="$t('端口')" width="100" />
      <el-table-column prop="limit" :label="$t('代理日限制')" width="100" />
      <el-table-column prop="password" :label="$t('密码')" />
      <el-table-column prop="username" :label="$t('用户名')" />
      <el-table-column :label="$t('操作')" width="200" align="center">
        <template #default="scope">
          <el-button size="small" @click="handleEdit(scope.row)">{{ $t('编辑') }}</el-button>
          <el-button size="small" type="danger" @click="handleDelete(scope.row.id)">{{ $t('删除') }}</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 新增/编辑模态框 -->
    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="600px">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px">
        <el-form-item label="代理名称" prop="name">
          <el-input v-model="form.name" placeholder="请输入代理名称" />
        </el-form-item>
        <el-form-item label="服务器地址" prop="server">
          <el-input v-model="form.server" placeholder="请输入SMTP服务器地址" />
        </el-form-item>
        <el-form-item label="端口" prop="port">
          <el-input v-model.number="form.port" placeholder="请输入端口号" type="number" />
        </el-form-item>
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" placeholder="请输入用户名" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="form.password" placeholder="请输入密码" />
        </el-form-item>
        <el-form-item label="代理日限制" prop="limit">
          <el-input v-model="form.limit" placeholder="请输入代理日限制" type="number" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, reactive, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import { Plus } from '@element-plus/icons-vue';

import { emailApi } from '@/api/email';

// 表格数据
const tableData = ref([]);

// 获取账号数据
const fetchEmailSmtp = async () => {
  const res = await emailApi.getEmailSmtpList()
  // 拦截器已解包，res 直接就是数据对象
  tableData.value = res.list || []
}

onMounted(() => {
  fetchEmailSmtp()
})


// 模态框状态
const dialogVisible = ref(false);
const dialogTitle = ref('新增邮件代理');
const formRef = ref(null);

// 表单数据
const form = reactive({
  id: '',
  name: '',
  server: '',
  port: '',
  username: '',
  password: '',
  limit: 50
});

// 表单验证规则
const rules = {
  name: [{ required: true, message: i18n.global.t('请输入代理名称'), trigger: 'blur' }],
  server: [{ required: true, message: i18n.global.t('请输入服务器地址'), trigger: 'blur' }],
  port: [
    { required: true, message: i18n.global.t('请输入端口号'), trigger: 'blur' },
    { type: 'number', message: i18n.global.t('端口号必须为数字'), trigger: 'blur' }
  ],
  username: [{ required: true, message: i18n.global.t('请输入用户名'), trigger: 'blur' }],
  password: [{ required: true, message: i18n.global.t('请输入密码'), trigger: 'blur' }],
  limit: [{ required: true, message: i18n.global.t('请输入代理日限制'), trigger: 'blur' }],
};

// 新增操作
const handleAdd = () => {
  dialogTitle.value = '新增邮件代理';
  Object.assign(form, { id: '', name: '', server: '', port: '', username: '', password: '', limit: 50 });
  dialogVisible.value = true;
};

// 编辑操作
const handleEdit = (row) => {
  dialogTitle.value = '编辑邮件代理';
  Object.assign(form, { ...row });
  dialogVisible.value = true;
};

// 删除操作
const handleDelete = (id) => {
  ElMessageBox.confirm('确定要删除该邮件代理吗？', '警告', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning'
  }).then( async () => {
      try {
        // 调用API删除账号
        const response = await emailApi.deleteEmailSmtp(id)
        // 拦截器已解包，response 直接就是数据对象
        ElMessage({
          type: 'success',
          message: i18n.global.t('删除成功'),
        })
        fetchEmailSmtp()
      } catch (error) {
        console.error('删除账号失败:', error)
        ElMessage.error(i18n.global.t('删除账号失败'))
      }
  });
};

// 提交表单
const  submitForm = () => {
  form.limit = Number(form.limit)
  formRef.value.validate(async (valid) => {
    if (valid) {
          if(form.id){
            const res = await emailApi.updateEmailSmtp(form.id,form)
            // 拦截器已解包，res 直接就是数据对象
            ElMessage.success(i18n.global.t('更新成功'))
            dialogVisible.value = false
            // 刷新账号列表
            fetchEmailSmtp()
          }else{
            const res = await emailApi.addEmailSmtp(form)
            // 拦截器已解包，res 直接就是数据对象
            ElMessage.success(i18n.global.t('新增成功'))
            dialogVisible.value = false
            // 刷新账号列表
            fetchEmailSmtp()
          }
      dialogVisible.value = false;
    }
  });
};
</script>

<style scoped>
.toolbar {
  margin-bottom: 16px;
  display: flex;
  justify-content: flex-end;
}

.email-settings {
  padding: 10px;
}
</style>