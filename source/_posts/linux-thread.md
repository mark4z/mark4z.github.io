---
title: Linux v5.19 risc-v 进程/线程调度汇编级源码解读
date: 2023-05-05 23:29:44
tags: [os, linux, cpu, thread, process, lwp]
---
`
不知不觉毕业已经四年有余。一直在做Java，做的再好终归是Spring上浅浅的一层苔藓，一刮就没了。
偶尔心血来潮看一眼《深入理解JVM虚拟机》也如过眼云烟，也始终不得要领，书籍繁多，却也没有一本是为门外汉准备的。
国内的书籍有种不鼓励自学的傲慢，喜欢假定读者已经有了一定的基础，可惜我是没有的，如果有的话我也不会再去看那本书了。
转而去读《OSTEP》，似进入桃花源的一小口，打开了[MIT6.S081]的大门。
听课的这段时间简直是工作后最快乐的时光，两位教授带领你把CPU/MM/FS层层剖开，边学边觉得知识从一个个孤岛融汇起来，知识在经脉里流淌。
终归，告别XV6之后还是要来Linux朝圣。
`

## 前言
本文并不是一篇Linux进程入门教程，建议按照顺序掌握以下知识后参考本文理解Linux源码。
- [x] Clang基础
- [x] OSTEP 操作系统导论 - 这绝对是一本好书，能让你低成本的思考操作系统的本质
- [x] [MIT6.S081](https://pdos.csail.mit.edu/6.S081/2022/tools.html) - MIT的本科操作系统课程，难度颇高，笔者花了两三个月的时间但很值得。
- [x] （可选）[Linux内核设计与实现](https://book.douban.com/subject/6097773/) - 本书是Linux内核的入门书籍，但是并算不上入门，对笔者来说有了6.S081的基础看起来仍旧很吃力。
- [x] [深入理解Linux进程管理(1.0)](https://blog.csdn.net/orangeboyye/article/details/125793172) - 开始入门linux
- [x] [深入理解Linux进程调度(0.4)](https://blog.csdn.net/orangeboyye/article/details/126109076) - 更具体的Linux进程调度，配合本文食用更佳。

## 线程调度的本质

对于Linux来说，与其说是进程调度，不如说是线程调度。本文会介绍Linux启动流程中与进程调度相关的准备工作以及线程切换的核心流程。

以一个简化的模型来看，linux运行在一个头尾相接的task数组的循环中。 当task运行完毕或者发生中断（时间片用完）时，重新选取下一个task运行。 如何选取下一个task，就是调度算法（例如CFS）。

从用户空间进程A的角度来看，自身一直在运行，但它运行在如下的循环中：
1. 运行
2. 发生时间中断，进入内核态并执行schedule()
3. 回到1
![](thread_switch_a.jpg)
然而，实际的魔法就发生在switch_to这里。从进程A的角度来看，执行了switch_to之后返回用户空间继续执行，好像什么都没有发生。

但从CPU的角度看，schedule()的前半部分发生在进程A的内核空间，执行了switch_to之后的后半部分发生在进程B的内核空间，然后在进程B的内核空间下，schedule()的后半部分被执行。

借由switch_to完成了一次偷天换日！这就是线程调度的本质。
![](thread_switch_b.jpg)

### 1号进程
在Linux中，0号进程和1号进程是两个非常重要的系统进程，它们扮演着系统启动和初始化的关键角色。

0号进程，也被称为调度进程或者Swapper进程，它是系统启动时创建的第一个进程。它的主要任务是创建1号进程，并在系统运行过程中负责处理僵尸进程。

1号进程，也被称为 init 进程，是系统启动后创建的第一个用户空间进程。它负责启动和监控所有其他的系统进程。如果一个进程的父进程结束，这个进程会被1号进程接管。因此，1号进程通常被视为所有进程的“父进程”。

这两个进程是系统初始化的核心组成部分，它们一起协同工作，确保系统能够正常启动，并开始执行用户空间的进程。虽然现代Linux发行版通常使用systemd或其他初始化系统来管理系统启动，但这两个进程的概念仍然存在，作为Linux系统的核心元素。

### 启动第一个用户空间程序
Linux在经过一系列的准备后，会fork出第一个用户空间进程，也就是1号进程，然后调用execve加载第一个用户空间程序，这个程序通常是/bin/init，也就是systemd。
分为以下几个步骤： 
#### 准备C环境，进入start_kernel()函数
```asm
_start(arch/riscv/kernel/head.S)
	/* jump to start kernel */
	j _start_kernel
_start_kernel(arch/riscv/kernel/head.S)
	/* Restore C environment */
	la tp, init_task 
	la sp, init_thread_union + THREAD_SIZE //初始化栈帧

	/* Start the kernel */
	call soc_early_init
	tail start_kernel //进入C环境
```

#### 进入start_kernel()函数，做一系列准备后创建1号进程
```c
start_kernel()(init/main.c)  
    /* Do the rest non-__init'ed, we're now alive */      
    arch_call_rest_init()(init/main.c)
        rest_init()(init/main.c)
            // fork出1号进程，指定fn为kernel_init(),kernel_init()会调用execve加载第一个用户空间程序init
            pid = user_mode_thread(kernel_init, NULL, CLONE_FS);(init/main.c)
                kernel_clone()(kernel/fork.c)
                    // 类似于xv6的fork_ret，在这里会提前把ret_from_kernel_thread放到task.thread.ra中
                    // 同时把kernel_init()放到task.thread.s0中。
                    // 在这里的p->thread是一个类似于xv6的Per-CPU context，不同的是，xv6保存在一个固定大小的数组中，而linux保存在task_struct中。
                    copy_process()(kernel/fork.c)
                        copy_thread()(arch/riscv/kernel/process.c)
                            p->thread.ra = (unsigned long)ret_from_kernel_thread;(arch/riscv/kernel/process.c)
                            p->thread.s[0] = (unsigned long)args->fn;
                            p->thread.s[1] = (unsigned long)args->fn_arg;
                    // task_struct准备好后，调用wake_up_new_task()将其加入到调度队列中
                    wake_up_new_task()(kernel/sched/core.c)
                        activate_task()(kernel/sched/core.c)
                            enqueue_task()(kernel/sched/core.c)
                                p->sched_class->enqueue_task(rq, p, flags);
```
此时1号进程已经创建好，并且已经入队，其中提前在task_struct中保存了kernel_init()的地址，准备第一次调度。

#### 第一次调度
```c
             /*
             * The boot idle thread must execute schedule()
             * at least once to get things moving:
             */
            schedule_preempt_disabled()(kernel/sched/core.c)
                // 调度器的核心函数，会调用pick_next_task()选取下一个task
                schedule()(kernel/sched/core.c)
                    __schedule()(kernel/sched/core.c)
                        // 选取下一个任务，这就是核心的调度算法，默认是CFS
                        next = pick_next_task(rq, prev, &rf);(kernel/sched/core.c)
                            // 进行线程切换，在这里会先切换页表，也就是修改stap寄存器。switch_mm_irqs_off()会调用switch_mm()
                            context_switch()(kernel/sched/core.c)
                                 /*
                                 * sys_membarrier() requires an smp_mb() between setting
                                 * rq->curr / membarrier_switch_mm() and returning to userspace.
                                 *
                                 * The below provides this either through switch_mm(), or in
                                 * case 'prev->active_mm == next->mm' through
                                 * finish_task_switch()'s mmdrop().
                                 */
                                //切换页表
                                switch_mm_irqs_off(prev->active_mm, next->mm, next);
                                // 保存Prev的寄存器到task.thread，然后恢复next的寄存器，这里等同于xv6的proc.context，也就是内核线程的context
                                //    /* CPU-specific state of a task */
                                //    struct thread_struct {
                                //        /* Callee-saved registers */
                                //        unsigned long ra;
                                //        unsigned long sp;	/* Kernel mode stack */
                                //        unsigned long s[12];	/* s[0]: frame pointer */
                                //        struct __riscv_d_ext_state fstate;
                                //        unsigned long bad_cause;
                                //    };
                                // a3 = prev->thread
                                // a4 = next->thread
                                switch_to()(arch/riscv/include/asm/switch_to.h)
                                    __switch_to(arch/riscv/kernel/entry.S)
                                        /* Save context into prev->thread */
                                        li    a4,  TASK_THREAD_RA
                                        add   a3, a0, a4
                                        add   a4, a1, a4
                                        REG_S ra,  TASK_THREAD_RA_RA(a3)
                                        REG_S sp,  TASK_THREAD_SP_RA(a3)
                                        REG_S s0,  TASK_THREAD_S0_RA(a3)
                                        REG_S s1,  TASK_THREAD_S1_RA(a3)
                                        ...
                                        /* Restore context from next->thread */
                                        REG_L ra,  TASK_THREAD_RA_RA(a4) //关键点，恢复next的ra，即ret_from_kernel_thread
                                        REG_L sp,  TASK_THREAD_SP_RA(a4) //恢复栈帧
                                        REG_L s0,  TASK_THREAD_S0_RA(a4) //恢复next的s0，即kernel_init()
                                        REG_L s1,  TASK_THREAD_S1_RA(a4) //恢复next的s1，即kernel_init()的args
                                        ...
                                        /* The offset of thread_info in task_struct is zero. */
                                        move tp, a1
                                        ret // 跳转至ret_from_kernel_thread
```
执行schedule_tail()，保存prev的相关状态，并跳转至kernel_init()。
```asm
ret_from_kernel_thread(arch/riscv/kernel/entry.S)
    schedule_tail()(kernel/sched/core.c)
    /* Call fn(arg) */
    la ra, ret_from_exception
    move a0, s1 // a0 = args->fn_arg
    jr s0       // s0 = args->fn = kernel_init() 跳转至kernel_init()
```

#### 加载INIT程序
kernel_execve()会调用load_elf_binary()加载init程序，然后调用start_thread()启动init程序。
```c
kernel_init()(init/main.c)
    try_to_run_init_process()(init/main.c)
        run_init_process()(init/main.c)
            kernel_execve()(fs/exec.c)
                bprm_execve()(fs/exec.c)
                    exec_binprm()(fs/exec.c)
                        search_binary_handler()(fs/exec.c)
                            retval = fmt->load_binary(bprm);(fs/exec.c)
                                load_elf_binary()(fs/binfmt_elf.c)
                                    //这里保存了init程序的用户空间的寄存器，等同于xv6的trapframe
                                    struct pt_regs *regs = current_pt_regs();
                                    // elf_entry是init程序的入口地址，即init程序的main函数。
                                	/* everything is now ready... get the userspace context ready to roll */
                                    START_THREAD(elf_ex, regs, elf_entry, bprm->p);
                                        regs->epc = pc;  // 设置epc为入口地址
                                        regs->sp = sp;   // 初始化栈帧
```
#### 回到用户空间
在ret_from_kernel_thread中执行kernel_init()前，已经设置了ra = ret_from_exception，
因此kernel_init()执行完毕后会跳转至ret_from_exception，然后执行resume_userspace()，最后执行sret回到用户空间。
至此，我们就真正开始运行1号进程init啦。
```c
```asm
ret_from_exception(arch/riscv/kernel/entry.S)    
// kernel_init()执行完毕后会跳转至此，此时s1不等于0，因此会继续运行到restore_all
resume_userspace: 
    /* Interrupts must be disabled here so flags are checked atomically */
    REG_L s0, TASK_TI_FLAGS(tp) // current_thread_info->flags
    andi s1, s0, _TIF_WORK_MASK
    bnez s1, work_pending
// 从task_struct.pt_regs中恢复所有的寄存器
restore_all:
	REG_L a0, PT_STATUS(sp)

	REG_L  a2, PT_EPC(sp) // a2 = epc
	REG_SC x0, a2, PT_EPC(sp)

	csrw CSR_STATUS, a0 
	csrw CSR_EPC, a2 // 设置epc为init程序的入口地址，这样sret后就会跳转至用户空间init程序的入口地址
    
	REG_L x1,  PT_RA(sp) 
	REG_L x3,  PT_GP(sp)
	REG_L x4,  PT_TP(sp)
	...

	REG_L x2,  PT_SP(sp)
	sret // 回到用户空间
```

### TODO 进程切换 timer interrupt

```c
handle_exception(arch/riscv/kernel/entry.S)
	la ra, ret_from_exception

	/* Handle interrupts */
	move a0, sp /* pt_regs */
	la a1, generic_handle_arch_irq
	jr a1
generic_handle_arch_irq()(kernel/irq/handle.c)
    riscv_intc_irq()(drivers/irqchip/irq-riscv-intc.c)
        generic_handle_domain_irq()(kernel/irq/irqdesc.c)
            handle_irq_desc()(kernel/irq/irqdesc.c)
                generic_handle_irq_desc(include/linux/irqdesc.h)
                    riscv_timer_interrupt()(drivers/clocksource/timer-riscv.c)
                    evdev->event_handler(evdev);
                        tick_handle_periodic()(kernel/time/tick-common.c)
                            tick_periodic()()(kernel/time/tick-common.c)
                                update_process_times()(kernel/time/timer.c)
                                    scheduler_tick()(kernel/sched/core.c)
                                        curr->sched_class->task_tick(rq, curr, 0);
                                        task_tick_fair()(kernel/sched/fair.c)
                                        /*
                                         * Update run-time statistics of the 'current'.
                                         */
                                        update_curr(cfs_rq);
                                        if (cfs_rq->nr_running > 1)
                                            check_preempt_tick(cfs_rq, curr);
                                            	ideal_runtime = sched_slice(cfs_rq, curr);
                                                delta_exec = curr->sum_exec_runtime - curr->prev_sum_exec_runtime;
                                                if (delta_exec > ideal_runtime) {
                                                    resched_curr(rq_of(cfs_rq));
                                                    /*
                                                     * The current task ran long enough, ensure it doesn't get
                                                     * re-elected due to buddy favours.
                                                     */
                                                    clear_buddies(cfs_rq, curr);
                                                    return;
                                                }
                                            check_preempt_tick()(kernel/sched/fair.c)
                                                resched_curr()(kernel/sched/core.c)
                                                    set_tsk_need_resched()(kernel/sched/core.c)
                                                        set_tsk_thread_flag(tsk,TIF_NEED_RESCHED);
ret_from_exception(arch/riscv/kernel/entry.S)
resume_userspace(arch/riscv/kernel/entry.S)
    andi s1, s0, _TIF_WORK_MASK
    bnez s1, work_pending
work_pending(arch/riscv/kernel/entry.S)   
	/* Enter slow path for supplementary processing */
	la ra, ret_from_exception
	andi s1, s0, _TIF_NEED_RESCHED
	bnez s1, work_resched 
work_resched(arch/riscv/kernel/entry.S)   
	tail schedule
```

假设另一个进程在运行时，timer interrupt发生，进程切换回init进程，然后会切换回__switch_to，至此，进程调度变成一个死循环。
