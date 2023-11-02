---
title: Linux v5.19 risc-v 进程调度汇编级源码解读
date: 2023-05-05 23:29:44
tags: [os, linux, cpu, thread, process, lwp]
---
`
不知不觉毕业已经四年有余。一直在做Java，做的再好终归是Spring上浅浅的一层苔藓，一刮就没了。
偶尔心血来潮看一眼《深入理解JVM虚拟机》也如过眼云烟，也始终不得要领，书籍繁多，却也没有一本是为门外汉准备的。
国内的书籍有种不鼓励自学的傲慢，喜欢假定读者已经有了一定的基础，可惜我是没有的，如果有的话我也不会再去看那本书了。
转而去读《OSTEP》，似进入桃花源的一小口，打开了[MIT6.S081](https://pdos.csail.mit.edu/6.S081/2022/tools.html)的大门。
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

## 进程调度的本质


Linux 0号进程fork 1号进程kernel_init，然后exec init process最终跳转回用户空间的全过程。

```c
_start(arch/riscv/kernel/head.S)
_start_kernel(arch/riscv/kernel/head.S)
start_kernel()(init/main.c)        
    arch_call_rest_init()(init/main.c)
        rest_init()(init/main.c)
            pid = user_mode_thread(kernel_init, NULL, CLONE_FS);(init/main.c)
                kernel_clone()(kernel/fork.c)
                    copy_process()(kernel/fork.c)
                        copy_thread()(arch/riscv/kernel/process.c)
                            p->thread.ra = (unsigned long)ret_from_kernel_thread;(arch/riscv/kernel/process.c)
                            p->thread.s[0] = (unsigned long)args->fn;
                            p->thread.s[1] = (unsigned long)args->fn_arg;
                    wake_up_new_task()(kernel/sched/core.c)
                        activate_task()(kernel/sched/core.c)
                            enqueue_task()(kernel/sched/core.c)
                                p->sched_class->enqueue_task(rq, p, flags);
            schedule_preempt_disabled()(kernel/sched/core.c)
                schedule()(kernel/sched/core.c)
                    __schedule()(kernel/sched/core.c)
                        next = pick_next_task(rq, prev, &rf);(kernel/sched/core.c)
                            context_switch()(kernel/sched/core.c)
                                switch_to()(arch/riscv/include/asm/switch_to.h)
                                    __switch_to(arch/riscv/kernel/entry.S)
                                        li    a4,  TASK_THREAD_RA
                                        add   a4, a1, a4
                                        REG_L ra,  TASK_THREAD_RA_RA(a4)
                                        REG_L s0,  TASK_THREAD_S0_RA(a4)
                                        REG_L s1,  TASK_THREAD_S1_RA(a4)
                                        ret
ret_from_kernel_thread(arch/riscv/kernel/entry.S)
    schedule_tail()(kernel/sched/core.c)
    /* Call fn(arg) */
    la ra, ret_from_exception
    move a0, s1
    jr s0
kernel_init()(init/main.c)
    try_to_run_init_process()(init/main.c)
        run_init_process()(init/main.c)
            kernel_execve()(fs/exec.c)
                bprm_execve()(fs/exec.c)
                    exec_binprm()(fs/exec.c)
                        search_binary_handler()(fs/exec.c)
                            retval = fmt->load_binary(bprm);(fs/exec.c)
                                load_elf_binary()(fs/binfmt_elf.c)
                                    start_thread()(arch/riscv/kernel/process.c)
                                        regs->epc = pc;
                                        regs->sp = sp;
ret_from_exception(arch/riscv/kernel/entry.S)    
resume_userspace
restore_all:
	REG_L  a2, PT_EPC(sp)
	REG_SC x0, a2, PT_EPC(sp)
    ...
	csrw CSR_STATUS, a0
	csrw CSR_EPC, a2
    REG_L x2,  PT_SP(sp)
    sret
```


timer interrupt 进程切换

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