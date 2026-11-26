part of 'post_screens.dart';

mixin _PersonalProfile on State<PersonalPostsScreen> {
  _PostAuthority get _authority;
  ScrollController get _scroll;
  Future<void> _open(PersonalPostItem item);
  Future<void> _deleteDraft(PersonalPostItem item);
  Future<void> _deleteFailedTask(PersonalPostItem item);
  void _chooseChannel();
  int _tab = 0;
  bool _repostsOpened = false;

  // 页签切换动画：内容先向切换方向平移 22% 并淡出，新内容反向滑入（原型 switchTab）。
  // 逐帧绘制由 RenderSliverTransform 直接监听 controller 完成，不逐帧 setState。
  // _tabSelected 是页签高亮（点击立即切换）；_tab 是内容页签（动画中点切换）。
  late final AnimationController _tabSwitch;
  late final CurvedAnimation _tabMotion;
  bool _tabSwitching = false, _tabPhaseOut = true;
  int _tabSelected = 0, _tabDir = 1, _tabTarget = 0;
  double _tabDragX = 0;
  bool _dragFromHero = false;
  final GlobalKey _heroKey = GlobalKey();

  void initProfileMotion(TickerProvider vsync) {
    _tabSwitch = AnimationController(
      vsync: vsync,
      duration: const Duration(milliseconds: 200),
    );
    // 页签内容切换与设置页转场共用 HhMotion.pageSlideCurve，观感一致。
    _tabMotion = CurvedAnimation(
      parent: _tabSwitch,
      curve: HhMotion.pageSlideCurve,
      reverseCurve: HhMotion.pageSlideCurve,
    );
    _tabSwitch.addStatusListener(_tabMotionStatus);
  }

  void disposeProfileMotion() {
    _tabSwitch.removeStatusListener(_tabMotionStatus);
    _tabMotion.dispose();
    _tabSwitch.dispose();
  }

  void _tabMotionStatus(AnimationStatus status) {
    if (status == AnimationStatus.completed) {
      // 先翻转入场相位再重建，确保本帧 rebuild 把新相位传给 RenderObject。
      _tabPhaseOut = false;
      _applyTab(_tabTarget);
      _tabSwitch.reverse();
    } else if (status == AnimationStatus.dismissed) {
      if (mounted) {
        setState(() => _tabSwitching = false);
      }
    }
  }

  void _selectTab(int index) {
    if (index < 0 || index > 4 || !mounted) {
      return;
    }
    if (index == _tabSelected) return;
    if (_tabSwitching) {
      // 连续滑动/点击：立即落在当前目标上（跳过剩余动画），马上开始下一
      // 次切换——每次手势都推进一个页签，不必等入场动画播完。
      _tabSwitch.stop();
      if (_tabTarget != _tab) _applyTab(_tabTarget);
      _tabPhaseOut = true;
      setState(() => _tabSwitching = false);
    }
    _tabTarget = index;
    _tabDir = index > _tab ? 1 : -1;
    if (HhMotion.reduced(context)) {
      _applyTab(index);
      return;
    }
    // 高亮立即跟随点击，内容做两段式动画；离场相位必须经 rebuild 传入
    // RenderSliverTransform。
    setState(() {
      _tabSelected = index;
      _tabSwitching = true;
      _tabPhaseOut = true;
    });
    _tabSwitch.forward(from: 0);
  }

  void _applyTab(int index) {
    // Threads 式：切页签只换内容，不记忆各页签旧位置，也不改动整体滚动
    // 位置；内容不足时会由布局收正，随后由下面的收口逻辑落到可达位置。
    // 注意：必须立即换内容——之前的“等数据到位再换”会把上一页签的内容
    // 残留在空页签下，并在数据到达时把滚动位置夹回顶部。
    final previous = _scroll.hasClients ? _scroll.offset : 0.0;
    setState(() {
      _tab = index;
      _tabSelected = index;
    });
    _restoreTabScrollOffset(previous);
    _startTabLoad(index);
  }

  /// 空页签占位：高度覆盖“页签栏以下的完整可用区域”，文字在空白区内居中。
  /// 高度不足会让空白页在滚动时被弹回顶部；高度变化时文字位置随之调整。
  Widget _emptySliver(BuildContext context, Widget child) => SliverLayoutBuilder(
        builder: (context, constraints) => SliverToBoxAdapter(
          child: SizedBox(
            height: (constraints.viewportMainAxisExtent - 56).clamp(
              0.0,
              double.infinity,
            ),
            child: Center(child: child),
          ),
        ),
      );

  /// 页签内容较少时补一段尾部空白，使可滚动范围至少达到“头像区收起、标签栏
  /// 固定”的位置；否则从深滚动位置切到内容少的页签会被夹回页面顶部。
  /// 采用 `precedingScrollExtent` 计算补多少，内容足够时不产生任何空白。
  Widget _tailFiller() => SliverLayoutBuilder(
    builder: (context, constraints) {
      final heroBox = _heroKey.currentContext?.findRenderObject();
      final hero = heroBox is RenderBox && heroBox.hasSize
          ? heroBox.size.height
          : 301.0;
      final needed =
          constraints.viewportMainAxisExtent +
          hero -
          constraints.precedingScrollExtent;
      if (needed <= 0) {
        return const SliverToBoxAdapter(child: SizedBox.shrink());
      }
      return SliverToBoxAdapter(child: SizedBox(height: needed));
    },
  );

  /// 页签内容是否已到位（含失败终态，避免无休止等待）。
  bool _tabReady(int index) {
    if (index == 0) return true;
    if (index == 1) return _replyLoaded;
    if (index == 2) return _repostsOpened;
    return _collections.containsKey(index) ||
        _collectionErrors.containsKey(index);
  }

  void _startTabLoad(int index) {
    if (index == 2 && !_repostsOpened) unawaited(_refreshReposts());
    if (index == 1 && !_replyLoaded) unawaited(_loadReplies());
    if (index >= 3 &&
        !_collections.containsKey(index) &&
        !_collectionErrors.containsKey(index)) {
      unawaited(_loadCollection(index));
    }
  }

  /// 等新页签内容实际布局完成后再补回原滚动位置：切换那一帧数据往往
  /// 尚未加载（可滚动范围过小），过早 jumpTo 会被夹到很小的值——这就是
  /// 「横滑切页签立刻跳回顶部」的根因。每帧重试直到范围足够或重试用尽
  /// （约 0.5 秒）；期间用户已开始手动滚动则放弃恢复，尊重用户操作。
  void _restoreTabScrollOffset(double target) {
    var retries = 30;
    void attempt() {
      if (!mounted || !_scroll.hasClients) return;
      final extent = _scroll.position.maxScrollExtent;
      // 目标页签还没就绪：等内容落定再一次性收口，避免先停在顶部再跳回。
      if (!_tabReady(_tab) && extent < target && retries > 0) {
        retries--;
        WidgetsBinding.instance.addPostFrameCallback((_) => attempt());
        return;
      }
      if (_scroll.position.isScrollingNotifier.value) return;
      _scroll.jumpTo(target.clamp(0.0, extent));
    }

    WidgetsBinding.instance.addPostFrameCallback((_) => attempt());
  }


  void _onProfileDragStart(DragStartDetails details) {
    // 以 hero 区（顶栏 + 头像资料块）底边为界区分横滑区域：
    // hero 区右滑打开设置；内容区左/右滑切换页签，帖签右滑打开设置。
    _dragFromHero = false;
    final box = _heroKey.currentContext?.findRenderObject();
    if (box is RenderBox && box.hasSize) {
      final heroBottom = box.localToGlobal(Offset.zero).dy + box.size.height;
      _dragFromHero = details.globalPosition.dy <= heroBottom;
    }
  }

  void _onProfileDragUpdate(DragUpdateDetails details) {
    _tabDragX += details.primaryDelta ?? 0;
  }

  void _onProfileDragEnd(DragEndDetails details) {
    final dx = _tabDragX;
    _tabDragX = 0;
    final velocity = details.primaryVelocity ?? 0;
    // 阈值适当放宽（原 72px/420px·s⁻¹），短滑/轻扫也能翻页。
    final forward = dx <= -56 || velocity <= -320;
    final back = dx >= 56 || velocity >= 320;
    if (_dragFromHero) {
      if (back) widget.onOpenSettings?.call();
      return;
    }
    if (forward) {
      _selectTab(_tabSelected + 1);
    } else if (back) {
      if (_tabSelected == 0) {
        widget.onOpenSettings?.call();
      } else {
        _selectTab(_tabSelected - 1);
      }
    }
  }

  Map<String, String> _repostIdentityNames = {};
  String? _repostIdentityError;

  Future<void> _loadRepostNames() async {
    try {
      final identities = await widget.controller.repostIdentities(
        authorityVersion: _authority.version,
      );
      if (!mounted || !_authority.current) return;
      setState(() {
        _repostIdentityNames = {
          for (final identity in identities) identity.id: identity.nickname,
        };
        _repostIdentityError = null;
      });
    } catch (_) {
      if (mounted && _authority.current) {
        setState(() => _repostIdentityError = '转帖身份读取失败，请重试');
      }
    }
  }

  Future<void> _refreshReposts() async {
    if (!_authority.current || !widget.controller.supportsReposts) return;
    _repostsOpened = true;
    await Future.wait([
      _loadRepostNames(),
      widget.controller.loadReposts(authorityVersion: _authority.version),
    ]);
  }

  PostDetail? _repostOriginal(String postId, String identityId) => widget
      .controller
      .repostItems
      .where((item) => item.postId == postId && item.identityId == identityId)
      .firstOrNull
      ?.post;

  Future<void> _confirmUndoRepost(String postId, String identityId) async {
    final c = widget.controller;
    if (!_authority.current || c.repostState(postId).unresolved) return;
    final confirmed = await showDialog<bool>(
      context: context,
      animationStyle: HhMotionRoutes.popup(context),
      builder: (dialogContext) => ListenableBuilder(
        listenable: c,
        builder: (context, __) => AlertDialog(
          backgroundColor: HhTelegramPalette.of(context).surface,
          surfaceTintColor: Colors.transparent,
          title: Text(
            hhText(context, '确认移除？'),
            style: TextStyle(color: HhTelegramPalette.of(context).ink),
          ),
          content: Text(
            hhText(
              context,
              '仅移除“{v0}”保存的这条记录。',
              args: {
                'v0':
                    _repostIdentityNames[identityId] ?? hhText(context, '此身份'),
              },
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(false),
              child: Text(hhText(context, '取消')),
            ),
            TextButton(
              key: ValueKey('confirm-undo-repost:$identityId/$postId'),
              onPressed: !_authority.current || c.repostState(postId).unresolved
                  ? null
                  : () {
                      if (!_authority.current ||
                          c.repostState(postId).unresolved) {
                        return;
                      }
                      Navigator.of(dialogContext).pop(true);
                    },
              child: Text(hhText(context, '确认')),
            ),
          ],
        ),
      ),
    );
    if (confirmed != true || !mounted || !_authority.current) return;
    await c.setRepost(
      postId,
      identityId,
      false,
      authorityVersion: _authority.version,
    );
  }

  Future<void> _showRepostMenu(String postId, String identityId) async {
    if (!_authority.current) return;
    final c = widget.controller;
    final governance = GovernanceScope.maybeOf(context);
    final action = await showModalBottomSheet<String>(
      context: context,
      sheetAnimationStyle: HhMotionRoutes.sheet(context),
      backgroundColor: HhTelegramPalette.of(context).surface,
      showDragHandle: true,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(28)),
      ),
      builder: (sheetContext) => SafeArea(
        child: ListenableBuilder(
          listenable: c,
          builder: (context, __) {
            final available =
                _authority.current &&
                _repostOriginal(postId, identityId) != null;
            void choose(String value) {
              if (!_authority.current) return;
              if (value == 'undo' && c.repostState(postId).unresolved) return;
              if (value != 'undo' && !available) return;
              Navigator.of(sheetContext).pop(value);
            }

            return Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                ListTile(
                  key: ValueKey('view-repost:$identityId/$postId'),
                  leading: Icon(
                    Icons.article_outlined,
                    color: HhTelegramPalette.of(context).secondary,
                  ),
                  title: Text(
                    hhText(context, '查看帖子'),
                    style: TextStyle(color: HhTelegramPalette.of(context).ink),
                  ),
                  enabled: available,
                  onTap: () => choose('view'),
                ),
                ListTile(
                  key: ValueKey('report-repost:$identityId/$postId'),
                  leading: Icon(
                    Icons.flag_outlined,
                    color: HhTelegramPalette.of(context).secondary,
                  ),
                  title: Text(
                    hhText(context, '举报帖子'),
                    style: TextStyle(color: HhTelegramPalette.of(context).ink),
                  ),
                  enabled: available && governance?.available == true,
                  onTap: () => choose('report'),
                ),
                ListTile(
                  key: ValueKey('cancel-repost:$identityId/$postId'),
                  leading: Icon(
                    Icons.remove_circle_outline,
                    color: HhTelegramPalette.of(context).secondary,
                  ),
                  title: Text(
                    hhText(context, '移除'),
                    style: TextStyle(color: HhTelegramPalette.of(context).ink),
                  ),
                  enabled:
                      _authority.current && !c.repostState(postId).unresolved,
                  onTap: () => choose('undo'),
                ),
              ],
            );
          },
        ),
      ),
    );
    if (action == null || !mounted || !_authority.current) return;
    if (action == 'undo') {
      await _confirmUndoRepost(postId, identityId);
      return;
    }
    final post = _repostOriginal(postId, identityId);
    if (post == null) return;
    if (action == 'view') {
      await _open(
        PersonalPostItem(
          id: '$identityId/$postId',
          title: post.title,
          sortAt: post.publishedAt,
          post: post,
        ),
      );
    } else if (action == 'report' && governance?.available == true) {
      await Navigator.of(context).push<void>(
        MaterialPageRoute(
          builder: (_) => ReportScreen(
            controller: governance!,
            postId: postId,
            title: post.title,
          ),
        ),
      );
    }
  }

  List<Widget> _repostSlivers() {
    final c = widget.controller;
    if (!c.supportsReposts) {
      return [
        _emptySliver(context, Text(hhText(context, '转帖服务暂不可用'))),
      ];
    }
    final items = c.repostItems;
    return [
      if (c.repostListLoading)
        const SliverToBoxAdapter(child: LinearProgressIndicator()),
      if (_repostIdentityError case final error?)
        SliverToBoxAdapter(
          child: TextButton(
            onPressed: _loadRepostNames,
            child: Text(hhText(context, error)),
          ),
        ),
      for (final id in c.unresolvedRepostIds)
        SliverToBoxAdapter(
          child: ListTile(
            title: Text(
              c.repostState(id).sending
                  ? hhText(context, '正在保存转帖…')
                  : hhText(context, '转帖结果尚未确认'),
            ),
            trailing: TextButton(
              onPressed: c.repostState(id).sending
                  ? null
                  : () =>
                        c.retryRepost(id, authorityVersion: _authority.version),
              child: Text(hhText(context, '重试核对')),
            ),
          ),
        ),
      if (c.repostListError case final error?)
        SliverToBoxAdapter(
          child: TextButton(
            onPressed: _refreshReposts,
            child: Text(hhText(context, error)),
          ),
        ),
      if (!c.repostListLoading &&
          c.repostListError == null &&
          c.unresolvedRepostIds.isEmpty &&
          items.isEmpty)
        _emptySliver(context, Text(hhText(context, '暂无转帖'))),
      SliverList.builder(
        itemCount: items.length,
        findChildIndexCallback: (key) {
          final index = items.indexWhere(
            (item) => key == ValueKey('personal-repost:${item.key}'),
          );
          return index < 0 ? null : index;
        },
        itemBuilder: (context, index) {
          final item = items[index];
          final state = c.repostState(item.postId);
          final post = item.post;
          final name = _repostIdentityNames[item.identityId];
          return Column(
            key: ValueKey('personal-repost:${item.key}'),
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (post == null) ...[
                ListTile(
                  title: Text(hhText(context, '原帖已不可用')),
                  subtitle: Text(hhText(context, '无法查看原帖内容')),
                  trailing: IconButton(
                    tooltip: hhText(context, '帖子更多操作'),
                    icon: const Icon(Icons.more_horiz_rounded),
                    onPressed: () =>
                        _showRepostMenu(item.postId, item.identityId),
                  ),
                ),
                const Divider(height: 1),
              ] else
                _ProfilePost(
                  key: ValueKey('repost-original:${item.key}'),
                  onMenu: () => _showRepostMenu(item.postId, item.identityId),
                  repostTag: name == null
                      ? hhText(context, '转帖')
                      : hhText(context, '{v0} 转帖', args: {'v0': name}),
                  repostAction: IconButton(
                    key: ValueKey('repost-undo:${item.key}'),
                    tooltip: hhText(context, '移除'),
                    color: const Color(0xFF229ED9),
                    onPressed: state.unresolved
                        ? null
                        : () =>
                              _confirmUndoRepost(item.postId, item.identityId),
                    icon: const PostRepostIcon(),
                  ),
                  post: post,
                  initialDetail: post,
                  own: false,
                  controller: c,
                  loadImages: widget.loadImages,
                  onOpen: () {
                    if (!_authority.current) return;
                    _open(
                      PersonalPostItem(
                        id: item.key,
                        title: post.title,
                        sortAt: item.repostedAt,
                        post: post,
                      ),
                    );
                  },
                ),
              if (state.error != null && !state.unresolved)
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 16),
                  child: Text(hhText(context, state.error!)),
                ),
            ],
          );
        },
      ),
      if (c.repostCursor != null)
        SliverToBoxAdapter(
          child: TextButton(
            key: const ValueKey('reposts-load-more'),
            onPressed: c.repostListLoading || c.unresolvedRepostIds.isNotEmpty
                ? null
                : () => c.loadReposts(
                    authorityVersion: _authority.version,
                    more: true,
                  ),
            child: Text(hhText(context, '加载更多')),
          ),
        ),
    ];
  }

  String? _profileIdentityId;
  int _identityEpoch = 0;
  int _visibilityEpoch = 0;
  int _projectionEpoch = 0;
  final _profileIdentityAnchor = GlobalKey();
  bool _identityMenuOpen = false;
  Route<dynamic>? _identityMenuRoute;

  void _dismissProfileIdentityMenu() {
    final route = _identityMenuRoute;
    _identityMenuRoute = null;
    if (route == null) return;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (route.isActive) route.navigator?.removeRoute(route);
    });
  }

  void profileIdentityChanged() {
    final id = widget.controller.personalIdentity?.id;
    if (id == _profileIdentityId) return;
    _profileIdentityId = id;
    ++_identityEpoch;
    _dismissProfileIdentityMenu();
    _hot = _sorting = false;
    _replies.clear();
    _replyLoaded = _replyLoading = false;
    _replyCursor = null;
    _replyError = null;
    _clearPostCardCache();
    if (_tab == 1 && id != null) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted && _authority.current) unawaited(_loadReplies());
      });
    }
  }

  /// A committed account block changes what the server may return, but the
  /// pages under the block route keep their loaded cards. Drop every cached
  /// collection and reply page, then reload the tab the user is looking at.
  void authorVisibilityChanged() {
    final epoch = widget.controller.authorVisibilityEpoch;
    if (epoch == _visibilityEpoch) return;
    _visibilityEpoch = epoch;
    _replies.clear();
    _replyLoaded = _replyLoading = false;
    _replyCursor = null;
    _replyError = null;
    _collections.clear();
    _collectionCursors.clear();
    _collectionErrors.clear();
    _collectionLoading.clear();
    _clearPostCardCache();
    final tab = _tab;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted || !_authority.current || _tab != tab) return;
      if (tab == 1) unawaited(_loadReplies());
      if (tab >= 3) unawaited(_loadCollection(tab));
    });
  }

  /// Identity mutations can change author projections already embedded in
  /// loaded subpages. Reload the visible identity-derived page after closing.
  void personalProjectionsChanged() {
    final epoch = widget.controller.personalProjectionEpoch;
    if (epoch == _projectionEpoch) return;
    _projectionEpoch = epoch;
    _replies.clear();
    _replyLoaded = _replyLoading = false;
    _replyCursor = null;
    _replyError = null;
    _repostsOpened = false;
    _collections.clear();
    _collectionCursors.clear();
    _collectionErrors.clear();
    _collectionLoading.clear();
    _clearPostCardCache();
    final tab = _tab;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted || !_authority.current || _tab != tab) return;
      if (tab == 1) unawaited(_loadReplies());
      if (tab == 2) unawaited(_refreshReposts());
      if (tab >= 3) unawaited(_loadCollection(tab));
    });
  }

  Future<void> _chooseIdentity() async {
    if (_identityMenuOpen || !_authority.current) return;
    final controller = widget.controller;
    final epoch = _identityEpoch;
    final anchor =
        _profileIdentityAnchor.currentContext!.findRenderObject()! as RenderBox;
    final overlay =
        Navigator.of(context).overlay!.context.findRenderObject()! as RenderBox;
    final rect =
        anchor.localToGlobal(Offset.zero, ancestor: overlay) & anchor.size;
    final p = HhTelegramPalette.of(context);
    setState(() => _identityMenuOpen = true);
    final id = await showMenu<String>(
      context: context,
      positionBuilder: (context, constraints) {
        final current = anchor.attached && overlay.attached
            ? anchor.localToGlobal(Offset.zero, ancestor: overlay) & anchor.size
            : rect;
        return RelativeRect.fromRect(
          Rect.fromLTWH(current.right, current.bottom + 6, 0, 0),
          Offset.zero & constraints.biggest,
        );
      },
      constraints: const BoxConstraints(minWidth: 208, maxWidth: 260),
      color: p.surface,
      surfaceTintColor: Colors.transparent,
      elevation: 6,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: p.line),
      ),
      popUpAnimationStyle: HhMotionRoutes.popup(context),
      items: [
        PopupMenuItem<String>(
          enabled: false,
          height: 40,
          child: Builder(
            builder: (context) {
              _identityMenuRoute = ModalRoute.of(context);
              if (!mounted || !_authority.current || epoch != _identityEpoch) {
                _dismissProfileIdentityMenu();
                return const SizedBox.shrink();
              }
              return Text(
                hhText(context, '切换身份'),
                style: TextStyle(fontSize: 13, color: p.secondary),
              );
            },
          ),
        ),
        for (final identity in controller.personalIdentities)
          PopupMenuItem<String>(
            key: ValueKey('profile-identity-${identity.id}'),
            value: identity.id,
            height: 56,
            child: Row(
              children: [
                HhDmAvatar(
                  identity: DmIdentity(
                    id: identity.id,
                    name: identity.nickname,
                    avatar: identity.avatar,
                  ),
                  size: 30,
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    identity.nickname,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(fontSize: 15, color: p.ink),
                  ),
                ),
                if (identity.id == controller.personalIdentity?.id)
                  Icon(Icons.check_rounded, color: p.accent, size: 20),
              ],
            ),
          ),
      ],
    );
    _identityMenuRoute = null;
    if (!mounted) return;
    setState(() => _identityMenuOpen = false);
    if (id == null || !_authority.current || epoch != _identityEpoch) return;
    await controller.selectPersonalIdentity(id);
  }

  Widget _profileSettingsButton() {
    final p = HhTelegramPalette.of(context);
    return Material(
      color: p.surface,
      shape: CircleBorder(side: BorderSide(color: p.line)),
      clipBehavior: Clip.antiAlias,
      child: HhPressFeedback(
        builder: (context, states) => IconButton(
          key: const ValueKey('profile-settings'),
          tooltip: hhText(context, '打开设置'),
          onPressed: widget.onOpenSettings,
          statesController: states,
          style: IconButton.styleFrom(foregroundColor: p.ink),
          icon: const Icon(Icons.settings_outlined, size: 22),
        ),
      ),
    );
  }

  Widget _profileHero(String name) {
    final c = widget.controller;
    final p = HhTelegramPalette.of(context);
    final ready = c.personalIdentity != null;
    return Column(
      children: [
        HhDmAvatar(
          identity: DmIdentity(
            id: c.personalIdentity?.id ?? '',
            name: name,
            avatar: c.personalIdentity?.avatar ?? 'default-v1',
          ),
          size: 104,
        ),
        const SizedBox(height: 12),
        Text(
          name,
          key: const ValueKey('profile-hero-name'),
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: TextStyle(
            fontSize: 23,
            fontWeight: FontWeight.w600,
            color: p.ink,
          ),
        ),
        const SizedBox(height: 16),
        Wrap(
          alignment: WrapAlignment.center,
          spacing: 12,
          runSpacing: 8,
          children: [
            _profileHeroPill(
              key: const ValueKey('profile-hero-photo'),
              icon: Icons.photo_camera_outlined,
              label: '设置头像',
              onTap: ready ? widget.onSetAvatar : null,
            ),
            _profileHeroPill(
              key: const ValueKey('profile-hero-info'),
              icon: Icons.edit_outlined,
              label: '编辑昵称',
              onTap: ready ? widget.onEditIdentity : null,
            ),
          ],
        ),
      ],
    );
  }

  Widget _profileHeroPill({
    Key? key,
    required IconData icon,
    required String label,
    VoidCallback? onTap,
  }) {
    final p = HhTelegramPalette.of(context);
    return Tooltip(
      message: hhText(context, label),
      child: Material(
        key: key,
        color: p.surface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(20),
          side: BorderSide(color: p.line),
        ),
        clipBehavior: Clip.antiAlias,
        child: HhPressFeedback(
          builder: (context, states) => InkWell(
            statesController: states,
            onTap: onTap,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(icon, size: 18, color: p.secondary),
                  const SizedBox(width: 6),
                  Text(
                    hhText(context, label),
                    style: TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w500,
                      color: p.ink,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _profileIdentityChip(String name) {
    final c = widget.controller;
    Widget chip(bool otherUnread) => HhIdentitySwitcher(
      identity: DmIdentity(
        id: c.personalIdentity?.id ?? '',
        name: name,
        avatar: c.personalIdentity?.avatar ?? 'default-v1',
      ),
      anchor: _profileIdentityAnchor,
      controlKey: const ValueKey('personal-profile'),
      menuOpen: _identityMenuOpen,
      enabled: c.personalIdentities.isNotEmpty,
      otherUnread: otherUnread,
      onTap: _chooseIdentity,
    );
    final listenable = widget.otherIdentityUnread;
    if (listenable == null) return chip(false);
    return ValueListenableBuilder<bool>(
      valueListenable: listenable,
      builder: (context, value, _) => chip(value),
    );
  }

  bool _hot = false, _sorting = false;
  final _replies = <CommentRow>[];
  bool _replyLoading = false, _replyLoaded = false;
  String? _replyCursor;
  String? _replyError;
  final _collections = <int, List<PostCard>>{};
  final _collectionCursors = <int, String?>{};
  final _collectionErrors = <int, String>{};
  final _collectionLoading = <int>{};

  // 页签内容本身按页签缓存；卡片级详情、互动与本人能力原来会在每次
  // 切回页签时随卡片重建各发一次请求，这里按帖子复用以保留页面状态。
  final _postDetails = <String, PostDetail>{};
  final _postCapabilities = <String, PostCapabilities>{};
  int _postCacheEpoch = 0;
  void _clearPostCardCache() {
    _postCacheEpoch++;
    _postDetails.clear();
    _postCapabilities.clear();
  }

  Future<void> _loadCollection(int tab, {bool more = false}) async {
    if (_collectionLoading.contains(tab) || !_authority.current) return;
    final visibilityEpoch = widget.controller.authorVisibilityEpoch;
    final projectionEpoch = widget.controller.personalProjectionEpoch;
    setState(() {
      _collectionLoading.add(tab);
      _collectionErrors.remove(tab);
    });
    try {
      final page = await widget.controller.loadPersonalCollection(
        favorites: tab == 4,
        cursor: more ? _collectionCursors[tab] : null,
      );
      if (!mounted ||
          !_authority.current ||
          widget.controller.authorVisibilityEpoch != visibilityEpoch ||
          widget.controller.personalProjectionEpoch != projectionEpoch) {
        return;
      }
      await widget.controller.loadInteractions(
        page.items.map((e) => e.postId).toList(),
      );
      if (!mounted ||
          !_authority.current ||
          widget.controller.authorVisibilityEpoch != visibilityEpoch ||
          widget.controller.personalProjectionEpoch != projectionEpoch) {
        return;
      }
      setState(() {
        final items = more
            ? List<PostCard>.of(_collections[tab] ?? [])
            : <PostCard>[];
        final seen = items.map((p) => p.postId).toSet();
        items.addAll(page.items.where((p) => seen.add(p.postId)));
        _collections[tab] = items;
        _collectionCursors[tab] = page.nextCursor;
      });
    } catch (_) {
      if (mounted &&
          _authority.current &&
          widget.controller.authorVisibilityEpoch == visibilityEpoch &&
          widget.controller.personalProjectionEpoch == projectionEpoch) {
        setState(() => _collectionErrors[tab] = '暂时无法读取，请重试');
      }
    } finally {
      if (mounted &&
          widget.controller.authorVisibilityEpoch == visibilityEpoch &&
          widget.controller.personalProjectionEpoch == projectionEpoch) {
        setState(() => _collectionLoading.remove(tab));
      }
    }
  }

  List<PostCard> _collectionItems(int tab) => (_collections[tab] ?? []).where((
    p,
  ) {
    if (widget.controller.isPostDeleted(p.postId)) return false;
    if (!widget.controller.supportsInteractions) return true;
    final value = widget.controller.interaction(p.postId).value;
    return value == null || (tab == 4 ? value.favoritedByMe : value.likedByMe);
  }).toList();

  void disposeProfile() => _dismissProfileIdentityMenu();

  Future<void> _sort(bool hot) async {
    if (hot && !widget.controller.supportsInteractions) {
      showHhFeedback(context, '热度数据暂不可用');
      return;
    }
    if (_sorting || widget.controller.personalLoading) return;
    final identityEpoch = _identityEpoch;
    setState(() {
      _hot = hot;
      _sorting = hot;
    });
    if (!hot) return;
    try {
      while (mounted &&
          _authority.current &&
          identityEpoch == _identityEpoch &&
          widget.controller.personalHasMore) {
        await widget.controller.loadPersonal(more: true);
        if (widget.controller.personalError != null) throw StateError('读取失败');
      }
      if (!mounted || identityEpoch != _identityEpoch) return;
      final ids = widget.controller.personalItems
          .where((e) => e.post != null)
          .map((e) => e.post!.postId)
          .toList();
      for (var i = 0; i < ids.length; i += 20) {
        await widget.controller.loadInteractions(
          ids.sublist(i, (i + 20).clamp(0, ids.length)),
        );
      }
      if (ids.any((id) => widget.controller.interaction(id).error != null)) {
        throw StateError('热度读取失败');
      }
    } catch (_) {
      if (mounted && _authority.current && identityEpoch == _identityEpoch) {
        setState(() => _hot = false);
        showHhFeedback(context, '暂时无法获取完整热度，请重试');
      }
    } finally {
      if (mounted && identityEpoch == _identityEpoch) {
        setState(() => _sorting = false);
      }
    }
  }

  Future<void> _loadReplies({bool more = false}) async {
    final identityId = widget.controller.personalIdentity?.id;
    if (_replyLoading || identityId == null || (more && _replyCursor == null)) {
      return;
    }
    final identityEpoch = _identityEpoch;
    final visibilityEpoch = widget.controller.authorVisibilityEpoch;
    final projectionEpoch = widget.controller.personalProjectionEpoch;
    bool currentVisibility() =>
        visibilityEpoch == widget.controller.authorVisibilityEpoch &&
        projectionEpoch == widget.controller.personalProjectionEpoch;
    final runtime = CommentScope.maybeOf(context);
    if (runtime == null) {
      setState(() => _replyError = '回复服务暂不可用');
      return;
    }
    setState(() {
      _replyLoading = true;
      _replyError = null;
    });
    final epoch = runtime.authorityVersion;
    try {
      final rows = <CommentRow>[];
      {
        final cursor = more ? _replyCursor : null;
        if (!mounted ||
            !_authority.current ||
            !runtime.current(epoch) ||
            !currentVisibility() ||
            identityEpoch != _identityEpoch) {
          return;
        }
        final page = await runtime.request(
          'GET',
          '/api/v1/me/comments',
          query: {
            'identityId': identityId,
            if (cursor != null) 'cursor': cursor,
          },
        );
        rows.addAll((page['items'] as List).map(CommentRow.new));
        if (!mounted ||
            !_authority.current ||
            !runtime.current(epoch) ||
            !currentVisibility() ||
            identityEpoch != _identityEpoch) {
          return;
        }
        final next = page['nextCursor'] as String?;
        if (next != null && next == cursor) {
          throw const FormatException('Reply cursor did not advance');
        }
        _replyCursor = next;
      }
      if (!mounted ||
          !_authority.current ||
          !runtime.current(epoch) ||
          !currentVisibility() ||
          identityEpoch != _identityEpoch) {
        return;
      }
      rows.sort(
        (a, b) => (b.data['publishedAt'] as String).compareTo(
          a.data['publishedAt'] as String,
        ),
      );
      setState(() {
        if (!more) _replies.clear();
        final ids = _replies.map((r) => r.id).toSet();
        _replies.addAll(
          rows.where(
            (r) =>
                !runtime.deleted.contains(r.id) &&
                !runtime.deleted.contains(r.root) &&
                ids.add(r.id),
          ),
        );
        _replyLoaded = true;
      });
    } catch (_) {
      if (mounted &&
          _authority.current &&
          currentVisibility() &&
          identityEpoch == _identityEpoch) {
        setState(() => _replyError = '回复加载失败，点击重试');
      }
    } finally {
      if (mounted && currentVisibility() && identityEpoch == _identityEpoch) {
        setState(() => _replyLoading = false);
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (mounted &&
              currentVisibility() &&
              identityEpoch == _identityEpoch &&
              _tab == 1 &&
              _replyError == null &&
              _replyCursor != null &&
              _scroll.hasClients &&
              _scroll.position.extentAfter < 320) {
            unawaited(_loadReplies(more: true));
          }
        });
      }
    }
  }

  Widget _pendingCard(PostController c, PersonalPostItem item) => Padding(
    padding: const EdgeInsets.fromLTRB(12, 6, 12, 6),
    child: _PersonalCard(
      key: ValueKey(item.id),
      item: item,
      controller: c,
      channelName: widget.channelNames[item.channelId] ?? '原分类',
      onOpen: () => _open(item),
      onDeleteDraft: item.isDraft ? () => _deleteDraft(item) : null,
      onDeleteTask:
          item.task?.canHide == true &&
              !{'SUBMITTED', 'UNKNOWN', 'RESULT_EXPIRED'}.contains(item.state)
          ? () => _deleteFailedTask(item)
          : null,
    ),
  );

  Widget _profileContent() {
    final c = widget.controller;
    final name =
        c.personalIdentity?.nickname ??
        (c.personalProfileError != null
            ? hhText(context, '个人资料暂不可用')
            : c.personalProfileLoading
            ? hhText(context, '正在加载个人资料…')
            : hhText(context, '我的身份'));
    final items = c.personalItems
        .where((e) => c.personalIdentity != null)
        .toList();
    if (_hot) {
      items.sort((a, b) {
        if (a.post == null || b.post == null) {
          return a.post == null
              ? (b.post == null ? b.sortAt.compareTo(a.sortAt) : -1)
              : 1;
        }
        int score(PersonalPostItem item) {
          final v = c.interaction(item.post!.postId).value;
          return (v?.likeCount ?? 0) +
              (v?.commentCount ?? 0) +
              (v?.favoriteCount ?? 0);
        }

        final result = score(b).compareTo(score(a));
        return result == 0 ? b.sortAt.compareTo(a.sortAt) : result;
      });
    }
    // 未完成/未发布优先，多条时先合成一叠；展开后最多先给三张，其余按需再展开。
    final pendingItems = items.where((e) => e.post == null).toList();
    final publishedItems = items.where((e) => e.post != null).toList();
    return RefreshIndicator(
      onRefresh: () async {
        _clearPostCardCache();
        if (_tab == 2) {
          await _refreshReposts();
        } else if (_tab == 1) {
          await _loadReplies();
        } else if (_tab >= 3) {
          await _loadCollection(_tab);
        } else {
          await c.loadPersonal();
          if (_hot) await _sort(true);
        }
      },
      child: NotificationListener<ScrollNotification>(
        onNotification: (notice) {
          if (_tab == 1 &&
              notice.metrics.extentAfter < 320 &&
              !_replyLoading &&
              _replyError == null &&
              _replyCursor != null) {
            unawaited(_loadReplies(more: true));
          }
          return false;
        },
        // 横滑识别阈值加倍：让竖向滚动优先，明确横滑才切页签/开设置，
        // 上下滑动与下拉刷新互不干扰。
        child: RawGestureDetector(
          behavior: HitTestBehavior.translucent,
          gestures: {
            _ProfileHorizontalDragRecognizer:
                GestureRecognizerFactoryWithHandlers<
                  _ProfileHorizontalDragRecognizer
                >(() => _ProfileHorizontalDragRecognizer(), (recognizer) {
                  recognizer
                    ..onStart = _onProfileDragStart
                    ..onUpdate = _onProfileDragUpdate
                    ..onEnd = _onProfileDragEnd;
                }),
          },
          child: CustomScrollView(
            controller: _scroll,
            physics: const AlwaysScrollableScrollPhysics(),
            slivers: [
              SliverToBoxAdapter(
                child: Padding(
                  key: _heroKey,
                  padding: const EdgeInsets.fromLTRB(24, 8, 24, 24),
                  child: Column(
                    children: [
                      Row(
                        children: [
                          _profileSettingsButton(),
                          const Spacer(),
                          Flexible(
                            flex: 3,
                            child: Align(
                              alignment: Alignment.centerRight,
                              child: _profileIdentityChip(name),
                            ),
                          ),
                        ],
                      ),
                      const SizedBox(height: 16),
                      _profileHero(name),
                    ],
                  ),
                ),
              ),
              SliverPersistentHeader(
                pinned: true,
                delegate: _ProfileTabs(
                  selected: _tabSelected,
                  hot: _hot,
                  onSort: _sort,
                  onSelect: _selectTab,
                ),
              ),
              _SliverTransform(
                animation: _tabMotion,
                phaseOut: _tabPhaseOut,
                direction: _tabDir,
                child: SliverMainAxisGroup(
                  slivers: [
                    if (_sorting)
                      const SliverToBoxAdapter(
                        child: LinearProgressIndicator(),
                      ),
                    if (_tab == 0) ...[
                      if (c.personalLoading && items.isEmpty)
                        SliverFillRemaining(
                          hasScrollBody: false,
                          child: Center(
                            child: CircularProgressIndicator(
                              semanticsLabel: hhText(context, '正在加载自己的帖子'),
                            ),
                          ),
                        )
                      else if (items.isEmpty)
                        _emptySliver(
                          context,
                          _PageState(
                            title: c.personalError != null
                                ? hhText(context, '暂时无法加载自己的帖子')
                                : hhText(context, '还没有自己的帖子'),
                            message: c.personalError,
                            action: c.personalError != null ? '重新加载' : '选择分类发帖',
                            actionIcon: c.personalError == null
                                ? Icons.add
                                : null,
                            onAction: c.personalError != null
                                ? c.loadPersonal
                                : _chooseChannel,
                          ),
                        ),
                      if (pendingItems.isNotEmpty)
                        SliverToBoxAdapter(
                          child: _PendingSection(
                            items: pendingItems,
                            channelNames: widget.channelNames,
                            cardBuilder: (item) => _pendingCard(c, item),
                            onOpen: _open,
                          ),
                        ),
                      SliverList.builder(
                        itemCount: publishedItems.length,
                        itemBuilder: (context, index) {
                          final item = publishedItems[index];
                          final cacheEpoch = _postCacheEpoch;
                          return _ProfilePost(
                            key: ValueKey(item.id),
                            post: item.post!,
                            controller: c,
                            loadImages: widget.loadImages,
                            initialDetail: _postDetails[item.post!.postId],
                            initialCapabilities:
                                _postCapabilities[item.post!.postId],
                            onDetailLoaded: (detail) {
                              if (cacheEpoch == _postCacheEpoch) {
                                _postDetails[item.post!.postId] = detail;
                              }
                            },
                            onCapabilitiesLoaded: (caps) {
                              if (cacheEpoch == _postCacheEpoch && caps != null) {
                                _postCapabilities[item.post!.postId] = caps;
                              }
                            },
                            onOpen: () => _open(item),
                          );
                        },
                      ),
                      if (items.isNotEmpty)
                        SliverToBoxAdapter(
                          child: _LoadMore(
                            loading: c.personalLoading,
                            hasMore: c.personalHasMore,
                            onPressed: () => c.loadPersonal(more: true),
                          ),
                        ),
                    ] else if (_tab == 1) ...[
                      if (_replyLoading)
                        const SliverToBoxAdapter(
                          child: LinearProgressIndicator(),
                        ),
                      if (_replyError case final error?
                          when _replies.isEmpty && !_replyLoading)
                        _emptySliver(
                          context,
                          Column(
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              Text(
                                hhText(context, error),
                                textAlign: TextAlign.center,
                              ),
                              const SizedBox(height: 4),
                              TextButton(
                                onPressed: () => _loadReplies(),
                                child: Text(hhText(context, '重试')),
                              ),
                            ],
                          ),
                        )
                      else if (_replyError != null)
                        SliverToBoxAdapter(
                          child: TextButton(
                            onPressed: () => _loadReplies(
                              more: _replyLoaded && _replyCursor != null,
                            ),
                            child: Text(hhText(context, _replyError!)),
                          ),
                        ),
                      if (!_replyLoading &&
                          _replies.isEmpty &&
                          _replyError == null)
                        _emptySliver(
                          context,
                          Text(hhText(context, '还没有回复')),
                        ),
                      SliverList.builder(
                        itemCount: _replies.length,
                        itemBuilder: (context, index) {
                          final row = _replies[index];
                          void openReply() => Navigator.push(
                            context,
                            MaterialPageRoute<void>(
                              builder: (_) => PostDetailScreen(
                                controller: c,
                                postId: row.post,
                                channelNames: widget.channelNames,
                                initialCommentId: row.id,
                                loadImages: widget.loadImages,
                                onAuthenticationRequired:
                                    widget.onAuthenticationRequired,
                              ),
                            ),
                          );
                          return PostFlatCard(
                            key: ValueKey('reply-${row.id}'),
                            onOpen: openReply,
                            onMenu: openReply,
                            author: row.nickname,
                            elapsed: postDateLabel(
                              context,
                              DateTime.parse(row.data['publishedAt'] as String),
                            ),
                            avatar: DetailAvatar(
                              name: row.nickname,
                              avatar: row.avatar,
                              radius: 15,
                            ),
                            body: row.body,
                            media:
                                row.data['image'] != null &&
                                    CommentScope.maybeOf(context) != null
                                ? CommentImageView(
                                    runtime: CommentScope.maybeOf(context)!,
                                    row: row,
                                  )
                                : null,
                            actions: [
                              PostFlatAction(
                                icon: Icons.chat_bubble_outline_rounded,
                                label: row.replyCount.toString(),
                                onTap: openReply,
                              ),
                              PostFlatAction(
                                icon: row.liked
                                    ? Icons.favorite_rounded
                                    : Icons.favorite_border_rounded,
                                on: row.liked,
                                label: row.likes.toString(),
                                onTap: openReply,
                              ),
                            ],
                          );
                        },
                      ),
                    ] else if (_tab == 2)
                      ..._repostSlivers()
                    else ...[
                      if (_collectionLoading.contains(_tab) &&
                          !_collections.containsKey(_tab))
                        const SliverToBoxAdapter(
                          child: LinearProgressIndicator(),
                        ),
                      if (_collectionErrors[_tab] case final error?)
                        SliverToBoxAdapter(
                          child: TextButton(
                            onPressed: () => _loadCollection(_tab),
                            child: Text(hhText(context, error)),
                          ),
                        ),
                      if (!_collectionLoading.contains(_tab) &&
                          !_collectionErrors.containsKey(_tab) &&
                          _collectionItems(_tab).isEmpty)
                        _emptySliver(
                          context,
                          Text(
                            _tab == 3
                                ? hhText(context, '还没有喜欢的帖子')
                                : hhText(context, '还没有收藏的帖子'),
                          ),
                        ),
                      SliverList.builder(
                        itemCount: _collectionItems(_tab).length,
                        itemBuilder: (_, index) {
                          final post = _collectionItems(_tab)[index];
                          final cacheEpoch = _postCacheEpoch;
                          return _ProfilePost(
                            key: ValueKey(('collection', _tab, post.postId)),
                            post: post,
                            controller: c,
                            loadImages: widget.loadImages,
                            own: false,
                            initialDetail: _postDetails[post.postId],
                            onDetailLoaded: (detail) {
                              if (cacheEpoch == _postCacheEpoch) {
                                _postDetails[post.postId] = detail;
                              }
                            },
                            onOpen: () => _open(
                              PersonalPostItem(
                                id: post.postId,
                                title: post.title,
                                sortAt: post.publishedAt,
                                post: post,
                              ),
                            ),
                          );
                        },
                      ),
                      if (_collectionCursors[_tab] != null)
                        SliverToBoxAdapter(
                          child: _LoadMore(
                            loading: _collectionLoading.contains(_tab),
                            hasMore: true,
                            onPressed: () => _loadCollection(_tab, more: true),
                          ),
                        ),
                    ],
                    // 内容较少时补尾部空白，保证可滚动范围到“头像收起”的位置。
                    _tailFiller(),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _ProfileTabs extends SliverPersistentHeaderDelegate {
  _ProfileTabs({
    required this.selected,
    required this.hot,
    required this.onSelect,
    required this.onSort,
  });
  final int selected;
  final bool hot;
  final ValueChanged<int> onSelect;
  final ValueChanged<bool> onSort;
  @override
  double get minExtent => 56;
  @override
  double get maxExtent => 56;
  @override
  bool shouldRebuild(_ProfileTabs old) => true;
  @override
  Widget build(
    BuildContext context,
    double shrinkOffset,
    bool overlapsContent,
  ) {
    const labels = ['帖子', '回复', '转帖', '喜欢', '收藏'];
    const icons = [
      Icons.article_outlined,
      Icons.chat_bubble_outline,
      Icons.repeat,
      Icons.favorite_border,
      Icons.bookmark_border,
    ];
    return Material(
      color: HhTelegramPalette.of(context).surface,
      child: Stack(
        children: [
          Row(
            children: [
              for (var i = 0; i < labels.length; i++)
                Expanded(
                  child: Semantics(
                    selected: selected == i,
                    button: true,
                    label: hhText(context, labels[i]),
                    child: HhAnimatedSelection(
                      key: ValueKey('profile-tab-$i'),
                      selected: selected == i,
                      builder: (context, progress) {
                        final color = Color.lerp(
                          HhTelegramPalette.of(context).secondary,
                          HhTelegramPalette.of(context).accent,
                          progress,
                        )!;
                        // 原型 .ptab：图标 + 11px 文字常显；选中 accent + w600。
                        final label = SizedBox(
                          height: 56,
                          child: Center(
                            child: Column(
                              mainAxisSize: MainAxisSize.min,
                              children: [
                                if (i == 2)
                                  IconTheme(
                                    data: IconThemeData(color: color),
                                    child: PostRepostIcon(
                                      size: 16 + 5 * progress,
                                    ),
                                  )
                                else
                                  Icon(
                                    icons[i],
                                    size: 16 + 5 * progress,
                                    color: color,
                                  ),
                                const SizedBox(height: 2),
                                Row(
                                  mainAxisSize: MainAxisSize.min,
                                  children: [
                                    Text(
                                      hhText(context, labels[i]),
                                      style: TextStyle(
                                        fontSize: 11,
                                        fontWeight: FontWeight.lerp(
                                          FontWeight.w400,
                                          FontWeight.w600,
                                          progress,
                                        ),
                                        color: color,
                                      ),
                                    ),
                                    if (i == 0)
                                      Icon(
                                        Icons.keyboard_arrow_down,
                                        size: 13,
                                        color: color,
                                      ),
                                  ],
                                ),
                              ],
                            ),
                          ),
                        );
                        if (i == 0 && selected == 0) {
                          return PopupMenuButton<bool>(
                            popUpAnimationStyle: HhMotionRoutes.popup(context),
                            tooltip: hhText(context, '帖子排序'),
                            initialValue: hot,
                            onSelected: onSort,
                            padding: EdgeInsets.zero,
                            itemBuilder: (_) => [
                              CheckedPopupMenuItem(
                                value: false,
                                checked: !hot,
                                child: Builder(
                                  builder: (context) =>
                                      Text(hhText(context, '最新')),
                                ),
                              ),
                              CheckedPopupMenuItem(
                                value: true,
                                checked: hot,
                                child: Builder(
                                  builder: (context) =>
                                      Text(hhText(context, '最热')),
                                ),
                              ),
                            ],
                            child: label,
                          );
                        }
                        return HhPressFeedback(
                          builder: (context, states) => InkWell(
                            statesController: states,
                            onTap: () => onSelect(i),
                            child: label,
                          ),
                        );
                      },
                    ),
                  ),
                ),
            ],
          ),
          Positioned(
            left: 0,
            right: 0,
            bottom: 0,
            child: Container(
              height: .5,
              color: HhTelegramPalette.of(context).line,
            ),
          ),
          Positioned(
            left: 0,
            right: 0,
            bottom: 0,
            child: AnimatedAlign(
              alignment: Alignment(-1 + selected / 2, 0),
              duration: HhMotion.duration(
                context,
                const Duration(milliseconds: 240),
              ),
              curve: HhMotion.pageSlideCurve,
              child: FractionallySizedBox(
                widthFactor: .2,
                child: Center(
                  child: SizedBox(
                    width: 28,
                    height: 2,
                    child: ColoredBox(
                      key: const ValueKey('profile-tab-indicator'),
                      color: HhTelegramPalette.of(context).accent,
                    ),
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _ProfilePost extends StatefulWidget {
  const _ProfilePost({
    super.key,
    required this.post,
    required this.controller,
    required this.onOpen,
    this.loadImages,
    this.own = true,
    this.initialDetail,
    this.initialCapabilities,
    this.onDetailLoaded,
    this.onCapabilitiesLoaded,
    this.onMenu,
    this.repostAction,
    this.repostTag,
  });
  final PostDetail? initialDetail;
  final PostCapabilities? initialCapabilities;
  final ValueChanged<PostDetail>? onDetailLoaded;
  final ValueChanged<PostCapabilities?>? onCapabilitiesLoaded;
  final bool own;
  final PostCard post;
  final PostController controller;
  final VoidCallback onOpen;
  final VoidCallback? onMenu;
  final Widget? repostAction;
  final String? repostTag;
  final PostImagesLoader? loadImages;
  @override
  State<_ProfilePost> createState() => _ProfilePostState();
}

class _ProfilePostState extends State<_ProfilePost> {
  late final _authority = _PostAuthority(widget.controller);
  PostDetail? _detail;
  bool _failed = false, _canEdit = false, _canDelete = false;
  @override
  void initState() {
    super.initState();
    scheduleMicrotask(() {
      if (mounted) unawaited(_load());
    });
  }

  Future<void> _load({bool refresh = false}) async {
    PostDetail? detail = refresh ? null : (widget.initialDetail ?? _detail);
    if (detail == null) {
      detail = await widget.controller.loadDetail(widget.post.postId);
      if (detail != null) widget.onDetailLoaded?.call(detail);
    }
    if (!mounted || !_authority.current) return;
    setState(() {
      _detail = detail;
      _failed = detail == null;
    });
    // 首次加载后互动值已保存在控制器里；切回页签重建卡片时直接复用，
    // 仅在值缺失或已被外部写入标记为需要刷新时才重新读取。
    if (widget.controller.supportsInteractions) {
      final interaction = widget.controller.interaction(widget.post.postId);
      if (interaction.value == null || interaction.refreshPending) {
        await widget.controller.loadInteractions([
          widget.post.postId,
        ], single: true);
      }
    }
    // 编辑／删除能力只在自己的帖子入口查询，避免列表页为每张卡片多发一次请求。
    if (!widget.own) return;
    PostCapabilities? caps = widget.initialCapabilities;
    if (caps == null) {
      caps = await widget.controller.postCapabilities(widget.post.postId);
      widget.onCapabilitiesLoaded?.call(caps);
    }
    if (!mounted || !_authority.current) return;
    setState(() {
      _canEdit = caps?.canEdit ?? false;
      _canDelete = caps?.canDelete ?? false;
    });
  }

  Future<void> _editPost() async {
    if (!_authority.current || !_canEdit) return;
    final session = widget.controller.openPostEdit(widget.post.postId);
    if (session == null) return;
    final outcome = await Navigator.of(context).push<String>(
      MaterialPageRoute(
        builder: (_) => _PublishedPostEditScreen(
          session: session,
          controller: widget.controller,
          loadImages: widget.loadImages,
        ),
      ),
    );
    if (!mounted || !_authority.current || outcome == null) return;
    await _load(refresh: true);
    if (!mounted || !_authority.current) return;
    final edited = widget.controller.confirmedEdit(widget.post.postId);
    if (edited != null) widget.controller.refreshEditedPost(edited);
    if (outcome.isNotEmpty) {
      showHhFeedback(context, outcome);
    }
  }

  Future<void> _deletePost() async {
    if (!_authority.current || !_canDelete) return;
    final confirmed = await _confirmScoped(
      context,
      widget.controller,
      title: '确认删除这篇帖子？',
      message: '',
      action: '删除帖子',
    );
    if (!mounted || !_authority.current || confirmed != true) return;
    final deleted = await widget.controller.deletePost(widget.post.postId);
    if (!mounted || !_authority.current || deleted) return;
    showHhFeedback(context, widget.controller.errorMessage ?? '删除结果待核对');
  }

  @override
  Widget build(BuildContext context) {
    final edited = widget.controller.confirmedEdit(widget.post.postId);
    final post = edited ?? widget.post;
    final detail = edited ?? widget.initialDetail ?? _detail;
    final interaction = widget.controller.supportsInteractions
        ? widget.controller.interaction(post.postId)
        : PostInteractionState();
    final canChange =
        widget.controller.supportsInteractions && interaction.canChange;
    final author = localizedPostAuthor(context, post.author);
    final palette = HhTelegramPalette.of(context);
    String count(int? value) => value == null ? '—' : value.toString();
    final failed = _failed && detail == null;
    return PostFlatCard(
      onOpen: widget.onOpen,
      onMenu: widget.onMenu,
      menuWidget: widget.onMenu == null
          ? SizedBox(
              width: 40,
              height: 40,
              child: PostActionMenu(
                controller: widget.controller,
                postId: post.postId,
                reportTitle: post.title,
                canEdit: _canEdit,
                canDelete: _canDelete,
                color: palette.secondary,
                size: 20,
                onEdit: _editPost,
                onDelete: _deletePost,
              ),
            )
          : null,
      author: author,
      repostTag: widget.repostTag,
      elapsed: postDateLabel(context, post.publishedAt),
      avatar: DetailAvatar(
        name: author,
        avatar: post.author.avatar,
        radius: 15,
      ),
      title: post.title,
      body: detail?.body ?? '',
      media: failed
          ? null
          : detail != null && detail.imageCount > 0 && widget.loadImages != null
          ? PostImagesSection(
              postId: post.postId,
              loadImages: widget.loadImages!,
              authority: widget.controller,
              isCurrent: () =>
                  _authority.current &&
                  !widget.controller.isPostDeleted(post.postId),
              expectedCount: detail.imageCount,
              feedPreview: true,
              feedPreviewInset: 20,
            )
          : null,
      failed: failed,
      onRetry: _load,
      bleedMedia: true,
      actions: [
        PostFlatAction(
          key: ValueKey('profile-like-${post.postId}'),
          icon: interaction.liked
              ? Icons.favorite_rounded
              : Icons.favorite_border_rounded,
          on: interaction.liked,
          selectedColor: palette.error,
          label: count(interaction.likes),
          onTap: canChange
              ? () => unawaited(
                  widget.controller.changeInteraction(
                    post.postId,
                    PostInteractionAction.like,
                  ),
                )
              : null,
        ),
        PostFlatAction(
          key: ValueKey('profile-favorite-${post.postId}'),
          icon: interaction.favorited
              ? Icons.bookmark_rounded
              : Icons.bookmark_border_rounded,
          on: interaction.favorited,
          label: count(interaction.value?.favoriteCount),
          onTap: canChange
              ? () => unawaited(
                  widget.controller.changeInteraction(
                    post.postId,
                    PostInteractionAction.favorite,
                  ),
                )
              : null,
        ),
        PostFlatAction(
          key: ValueKey('profile-comment-${post.postId}'),
          glyph: FeedActionIcon(
            action: FeedAction.comment,
            color: palette.secondary,
            size: 17,
          ),
          label: count(interaction.value?.commentCount),
          onTap: widget.onOpen,
        ),
        if (widget.repostAction != null) widget.repostAction!,
      ],
      notice: interaction.writeError != null
          ? hhText(context, interaction.writeError!)
          : null,
      onNoticeRetry: interaction.sending
          ? null
          : () => widget.controller.retryInteraction(post.postId),
    );
  }
}

/// 「我的」页五页签的扁平帖子卡片（原型 .post：通栏白底、底部分割线）。
/// 「我的」页与首页共用的扁平帖子卡片（通栏白底 + 底部分割线）。
/// 首页独有内容（标签、浏览数、作者跳转）通过可选插槽复用同一版式。
class PostFlatCard extends StatelessWidget {
  const PostFlatCard({
    super.key,
    required this.onOpen,
    this.onMenu,
    this.menuWidget,
    required this.author,
    required this.elapsed,
    required this.avatar,
    required this.actions,
    this.onAvatar,
    this.onAuthor,
    this.tags,
    this.views,
    this.repostTag,
    this.title,
    this.body,
    this.media,
    this.bleedMedia = false,
    this.failed = false,
    this.onRetry,
    this.notice,
    this.onNoticeRetry,
  });
  final VoidCallback onOpen;
  final VoidCallback? onMenu;
  final Widget? menuWidget;
  final Widget avatar;
  final String author, elapsed;
  final String? title, body, repostTag, notice;
  final Widget? media;

  /// 头像／作者名点击（首页用于作者页与私信入口）。
  final VoidCallback? onAvatar, onAuthor;

  /// 标签行（首页）：一行可滚动的 #话题。
  final Widget? tags;

  /// 操作行左侧的浏览数等补充信息（首页）。
  final Widget? views;

  /// 图片区移出左右内边距，由媒体组件自己控制左右边界（多图出血）。
  final bool bleedMedia;
  final bool failed;
  final VoidCallback? onRetry, onNoticeRetry;
  final List<Widget> actions;
  Widget _tappable(VoidCallback? onTap, Widget child) =>
      onTap == null ? child : GestureDetector(onTap: onTap, child: child);
  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    final title = this.title;
    final body = this.body;
    return KeyedSubtree(
      key: const ValueKey('flat-post-card'),
      child: Material(
        color: p.surface,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            InkWell(
              onTap: onOpen,
              child: Padding(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 10),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    if (repostTag case final tag?)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 7),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(
                              Icons.repeat_rounded,
                              size: 14,
                              color: p.accent,
                            ),
                            const SizedBox(width: 5),
                            Flexible(
                              child: Text(
                                tag,
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                                style: TextStyle(fontSize: 12, color: p.accent),
                              ),
                            ),
                          ],
                        ),
                      ),
                    Row(
                      children: [
                        _tappable(onAvatar, avatar),
                        const SizedBox(width: 9),
                        Expanded(
                          child: _tappable(
                            onAuthor,
                            Wrap(
                              spacing: 6,
                              runSpacing: 2,
                              crossAxisAlignment: WrapCrossAlignment.center,
                              children: [
                                Text(
                                  author,
                                  maxLines: 1,
                                  overflow: TextOverflow.ellipsis,
                                  style: TextStyle(
                                    fontSize: 13,
                                    fontWeight: FontWeight.w500,
                                    color: p.ink,
                                  ),
                                ),
                                Text(
                                  elapsed,
                                  maxLines: 2,
                                  overflow: TextOverflow.ellipsis,
                                  style: TextStyle(
                                    fontSize: 12,
                                    color: p.secondary,
                                  ),
                                ),
                              ],
                            ),
                          ),
                        ),
                        const SizedBox(width: 6),
                        menuWidget ??
                            SizedBox(
                              width: 40,
                              height: 40,
                              child: IconButton(
                                tooltip: hhText(context, '帖子更多操作'),
                                padding: EdgeInsets.zero,
                                visualDensity: VisualDensity.compact,
                                icon: Icon(
                                  Icons.more_horiz_rounded,
                                  size: 18,
                                  color: p.secondary,
                                ),
                                onPressed: onMenu,
                              ),
                            ),
                      ],
                    ),
                    if (title != null && title.isNotEmpty) ...[
                      const SizedBox(height: 2),
                      Text(
                        title,
                        style: TextStyle(
                          fontSize: 16,
                          fontWeight: FontWeight.w600,
                          height: 1.5,
                          color: p.ink,
                        ),
                      ),
                      if (body != null && body.isNotEmpty)
                        const SizedBox(height: 4),
                    ],
                    if (body != null && body.isNotEmpty)
                      Text(
                        body,
                        maxLines: 8,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(
                          fontSize: 14,
                          height: 1.6,
                          color: p.ink.withValues(alpha: .88),
                        ),
                      ),
                    if (tags case final tags?) tags,
                    if (!bleedMedia && media != null) media!,
                    if (failed)
                      Padding(
                        padding: const EdgeInsets.only(top: 8),
                        child: GestureDetector(
                          onTap: onRetry,
                          child: Text(
                            hhText(context, '内容加载失败，点击重试'),
                            style: TextStyle(fontSize: 13, color: p.secondary),
                          ),
                        ),
                      ),
                  ],
                ),
              ),
            ),
            if (bleedMedia && media != null) media!,
            Padding(
              // 图片与操作行的间距和操作行到卡片底边的间距一致（各 6）。
              padding: const EdgeInsets.fromLTRB(20, 6, 20, 6),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.end,
                children: [
                  if (views case final views?) ...[
                    Flexible(child: views),
                    const Spacer(),
                  ],
                  for (var i = 0; i < actions.length; i++) ...[
                    if (i > 0) const SizedBox(width: 22),
                    actions[i],
                  ],
                ],
              ),
            ),
            if (notice case final notice?)
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
                child: Row(
                  children: [
                    Expanded(
                      child: Text(
                        notice,
                        style: TextStyle(fontSize: 12, color: p.error),
                      ),
                    ),
                    if (onNoticeRetry != null)
                      TextButton(
                        key: const ValueKey('profile-interaction-retry'),
                        onPressed: onNoticeRetry,
                        child: Text(
                          hhText(context, '重试核对'),
                          style: TextStyle(fontSize: 12, color: p.error),
                        ),
                      ),
                  ],
                ),
              ),
            Container(height: 1, color: p.line),
          ],
        ),
      ),
    );
  }
}

/// 卡片底部的轻量操作（评论、收藏、点赞）。
class PostFlatAction extends StatefulWidget {
  const PostFlatAction({
    super.key,
    this.icon,
    this.glyph,
    required this.label,
    this.on = false,
    this.selectedColor,
    this.semanticsLabel,
    this.onTap,
  });
  final IconData? icon;

  /// 与首页共用的自绘图标；优先于 [icon]。
  final Widget? glyph;
  final String label;
  final bool on;
  final Color? selectedColor;

  /// 读屏用完整描述（含数量）；为空时只读图标与数字。
  final String? semanticsLabel;
  final VoidCallback? onTap;
  @override
  State<PostFlatAction> createState() => PostFlatActionState();
}

class PostFlatActionState extends State<PostFlatAction>
    with SingleTickerProviderStateMixin {
  late final _pop = AnimationController(
    vsync: this,
    duration: HhMotion.quick,
  );
  late final _pulse = TweenSequence<double>([
    TweenSequenceItem(
      tween: Tween(
        begin: 1.0,
        end: 1.16,
      ).chain(CurveTween(curve: Curves.easeOut)),
      weight: 40,
    ),
    TweenSequenceItem(
      tween: Tween(
        begin: 1.16,
        end: 1.0,
      ).chain(CurveTween(curve: Curves.easeIn)),
      weight: 60,
    ),
  ]).animate(_pop);

  @override
  void dispose() {
    _pop.dispose();
    super.dispose();
  }

  void _handleTap() {
    final onTap = widget.onTap;
    if (onTap == null) return;
    // 轻量点击反馈：图标快速脉一下，数量变化由 HhAnimatedCount 滚动。
    if (!HhMotion.reduced(context)) _pop.forward(from: 0);
    onTap();
  }

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    final on = widget.on;
    final color = on ? widget.selectedColor ?? p.accent : p.secondary;
    final content = HhPressFeedback(
      enabled: widget.onTap != null,
      pressedScale: .92,
      builder: (context, states) => InkWell(
        statesController: states,
        onTap: widget.onTap == null ? null : _handleTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 2, vertical: 4),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              ScaleTransition(
                scale: _pulse,
                child:
                    widget.glyph ??
                    Icon(widget.icon!, size: 17, color: color),
              ),
              if (widget.label.isNotEmpty) ...[
                const SizedBox(width: 6),
                HhAnimatedCount(
                  text: widget.label,
                  style: TextStyle(fontSize: 12, color: color),
                ),
              ],
            ],
          ),
        ),
      ),
    );
    final label = widget.semanticsLabel;
    if (label == null) return content;
    return Semantics(
      button: true,
      enabled: widget.onTap != null,
      selected: on,
      label: label,
      excludeSemantics: true,
      onTap: widget.onTap == null ? null : _handleTap,
      child: content,
    );
  }
}

/// 「我的」页横滑识别器：按角度而非粗放距离阈值判定——净水平位移超过
/// 1.1 倍触点容差、且明显大于净垂直位移（|dx| > 1.1·|dy|，横向 ±42° 内）
/// 即接管，接近横向的斜滑也能翻页（此前 2 倍距离阈值会把斜滑让给竖滚，
/// 表现为“偶尔卡在某一页”）；纵向为主的滚动仍由竖向识别器优先接管。
class _ProfileHorizontalDragRecognizer extends HorizontalDragGestureRecognizer {
  _ProfileHorizontalDragRecognizer();

  Offset? _origin;
  Offset _delta = Offset.zero;

  @override
  void addPointer(PointerDownEvent event) {
    _origin = event.position;
    _delta = Offset.zero;
    super.addPointer(event);
  }

  @override
  void handleEvent(PointerEvent event) {
    final origin = _origin;
    if (origin != null && event is PointerMoveEvent) {
      _delta = event.position - origin;
    }
    super.handleEvent(event);
  }

  @override
  bool hasSufficientGlobalDistanceToAccept(
    PointerDeviceKind pointerDeviceKind,
    double? deviceTouchSlop,
  ) {
    final dx = _delta.dx.abs();
    return dx > computeHitSlop(pointerDeviceKind, gestureSettings) * 1.1 &&
        dx > _delta.dy.abs() * 1.1;
  }
}

/// 对一组 sliver 做水平平移与透明度绘制（页签切换动画），布局尺寸不变。
/// RenderObject 直接监听 controller 逐帧 markNeedsPaint，widget 只在相位
/// 切换（离场→入场、动画结束）时重建。
class _SliverTransform extends SingleChildRenderObjectWidget {
  const _SliverTransform({
    required this.animation,
    required this.phaseOut,
    required this.direction,
    required super.child,
  });
  final Animation<double> animation;
  final bool phaseOut;
  final int direction;
  @override
  RenderSliverTransform createRenderObject(BuildContext context) =>
      RenderSliverTransform(animation, phaseOut, direction);
  @override
  void updateRenderObject(
    BuildContext context,
    RenderSliverTransform renderObject,
  ) {
    renderObject
      ..animation = animation
      ..phaseOut = phaseOut
      ..direction = direction;
  }
}

class RenderSliverTransform extends RenderSliver
    with RenderObjectWithChildMixin<RenderSliver> {
  RenderSliverTransform(
    Animation<double> animation,
    bool phaseOut,
    int direction,
  ) : _animation = animation,
      _phaseOut = phaseOut,
      _direction = direction;
  Animation<double> _animation;
  bool _phaseOut;
  int _direction;

  void _tick() => markNeedsPaint();

  @override
  void attach(PipelineOwner owner) {
    super.attach(owner);
    _animation.addListener(_tick);
  }

  @override
  void detach() {
    _animation.removeListener(_tick);
    super.detach();
  }

  set animation(Animation<double> value) {
    if (_animation == value) return;
    if (attached) {
      _animation.removeListener(_tick);
      value.addListener(_tick);
    }
    _animation = value;
    markNeedsPaint();
  }

  set phaseOut(bool value) {
    if (_phaseOut == value) return;
    _phaseOut = value;
    markNeedsPaint();
  }

  set direction(int value) {
    if (_direction == value) return;
    _direction = value;
    markNeedsPaint();
  }

  @override
  void performLayout() {
    final child = this.child;
    if (child == null) {
      geometry = SliverGeometry.zero;
      return;
    }
    // parentUsesSize 必须为 true：缺失时子级(Group)成为自己的 relayout
    // boundary，内容加载后视口拿不到新的 scrollExtent（maxScrollExtent
    // 归零 → 上滑不动/整页 overscroll 拉伸，v8-v9 实测根因）。
    child.layout(constraints, parentUsesSize: true);
    geometry = child.geometry;
  }

  // 命中测试必须转发给子 sliver：默认实现返回 false 会吞掉内容区全部
  // 触摸事件（卡片点击、滚动条内交互全失效）。
  @override
  bool hitTestChildren(
    SliverHitTestResult result, {
    required double mainAxisPosition,
    required double crossAxisPosition,
  }) {
    final child = this.child;
    if (child == null || child.geometry == null || !child.geometry!.visible) {
      return false;
    }
    return result.addWithAxisOffset(
      mainAxisPosition: mainAxisPosition,
      crossAxisPosition: crossAxisPosition,
      paintOffset: Offset.zero,
      mainAxisOffset: 0,
      crossAxisOffset: 0,
      hitTest: child.hitTest,
    );
  }

  // 语义树/全局坐标换算需要与 paint 一致的水平平移；缺失会让 debug 构建在
  // flushSemantics 时抛错并污染整棵语义树（连续异常把帧拖垮）。
  @override
  void applyPaintTransform(RenderObject child, Matrix4 transform) {
    assert(child == this.child);
    final v = _animation.value;
    final fraction = (_phaseOut ? -_direction : _direction) * .22 * v;
    transform.translateByDouble(
      fraction * constraints.crossAxisExtent,
      0.0,
      0.0,
      1.0,
    );
  }

  @override
  void paint(PaintingContext context, Offset offset) {
    final child = this.child;
    if (child == null || geometry == null || geometry!.paintExtent == 0) {
      return;
    }
    final v = _animation.value;
    // 离场：内容向滑动方向推出并淡出（v: 0→1）；入场：反向滑回（v: 1→0）。
    final fraction = (_phaseOut ? -_direction : _direction) * .22 * v;
    final opacity = 1 - v;
    if (fraction == 0 && opacity >= 1) {
      context.paintChild(child, offset);
      return;
    }
    final dx = fraction * constraints.crossAxisExtent;
    context.pushTransform(
      opacity < 1,
      offset,
      Matrix4.translationValues(dx, 0, 0),
      (context, offset) {
        if (opacity < 1) {
          context.pushOpacity(
            offset,
            (opacity * 255).round(),
            (context, offset) => context.paintChild(child, offset),
          );
        } else {
          context.paintChild(child, offset);
        }
      },
    );
  }
}

/// 未完成发布集合的展开/收起共用同一按钮样式与按压动画。
class _PendingToggleButton extends StatefulWidget {
  const _PendingToggleButton({required this.expanded, required this.onPressed});
  final bool expanded;
  final VoidCallback onPressed;

  @override
  State<_PendingToggleButton> createState() => _PendingToggleButtonState();
}

class _PendingToggleButtonState extends State<_PendingToggleButton>
    with SingleTickerProviderStateMixin {
  late final _press = AnimationController.unbounded(vsync: this, value: 1);

  void _release() => _press.animateTo(
    1,
    duration: HhMotion.pressOut,
    curve: HhMotion.curve,
  );

  @override
  void dispose() {
    _press.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Listener(
      onPointerDown: (_) => _press.animateTo(
        .96,
        duration: HhMotion.pressIn,
        curve: HhMotion.curve,
      ),
      onPointerUp: (_) => _release(),
      onPointerCancel: (_) => _release(),
      child: ScaleTransition(
        scale: _press,
        child: TextButton(
          key: ValueKey(
            widget.expanded
                ? 'profile-pending-collapse'
                : 'profile-pending-expand',
          ),
          onPressed: widget.onPressed,
          style: TextButton.styleFrom(
            backgroundColor: Theme.of(context).colorScheme.primary,
            foregroundColor: Theme.of(context).colorScheme.onPrimary,
            visualDensity: VisualDensity.compact,
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(12),
            ),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                widget.expanded
                    ? Icons.expand_less_rounded
                    : Icons.expand_more_rounded,
                size: 18,
              ),
              const SizedBox(width: 8),
              Text(hhText(context, widget.expanded ? '收起' : '展开')),
            ],
          ),
        ),
      ),
    );
  }
}

/// 未完成、未发布帖子的收起态：多行紧凑摘要收在同一个容器里，每行可直接点开，底部按钮展开。
class _UnfinishedStack extends StatelessWidget {
  const _UnfinishedStack({
    required this.items,
    required this.channelNames,
    required this.onExpand,
    required this.onOpen,
  });
  final List<PersonalPostItem> items;
  final Map<String, String> channelNames;
  final VoidCallback onExpand;
  final void Function(PersonalPostItem) onOpen;

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    // 收起时只露最近三条，避免一屏全是同一类卡片。
    final preview = items.take(3).toList();
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 6, 12, 6),
      child: Material(
        color: p.raised,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(16),
          side: BorderSide(color: p.line),
        ),
        clipBehavior: Clip.antiAlias,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            for (var i = 0; i < preview.length; i++) ...[
              if (i > 0)
                Divider(height: .5, thickness: .5, color: p.line, indent: 56),
              _UnfinishedRow(
                item: preview[i],
                channelName: channelNames[preview[i].channelId] ?? '原分类',
                onTap: () => onOpen(preview[i]),
              ),
            ],
            Divider(height: .5, thickness: .5, color: p.line),
            Padding(
              padding: const EdgeInsets.fromLTRB(14, 6, 8, 6),
              child: Row(
                children: [
                  Text(
                    hhText(
                      context,
                      '共 {count} 条未完成',
                      args: {'count': items.length},
                    ),
                    style: TextStyle(fontSize: 12, color: p.secondary),
                  ),
                  const Spacer(),
                  _PendingToggleButton(expanded: false, onPressed: onExpand),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// 未完成帖子区：收起时露出最近三条摘要，展开时完整列出全部卡片。
///
/// 高度伸缩由显式 [AnimationController] 驱动，并设置 `AnimationBehavior.preserve`，
/// 避免被系统「移除动画 / 动画时长倍率为 0」或无障碍服务降级成瞬间切换。
class _PendingSection extends StatefulWidget {
  const _PendingSection({
    required this.items,
    required this.channelNames,
    required this.cardBuilder,
    required this.onOpen,
  });

  final List<PersonalPostItem> items;
  final Map<String, String> channelNames;
  final Widget Function(PersonalPostItem) cardBuilder;
  final void Function(PersonalPostItem) onOpen;

  @override
  State<_PendingSection> createState() => _PendingSectionState();
}

class _PendingSectionState extends State<_PendingSection> {
  bool _expanded = false;

  void _toggle({required bool expand}) {
    if (_expanded == expand) return;
    setState(() => _expanded = expand);
  }

  Widget _buildContent(BuildContext context) {
    if (_expanded) {
      return Column(
        children: [
          for (final item in widget.items) widget.cardBuilder(item),
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 0, 12, 0),
            child: Align(
              alignment: Alignment.centerRight,
              child: _PendingToggleButton(
                expanded: true,
                onPressed: () => _toggle(expand: false),
              ),
            ),
          ),
        ],
      );
    }
    return _UnfinishedStack(
      items: widget.items,
      channelNames: widget.channelNames,
      onExpand: () => _toggle(expand: true),
      onOpen: widget.onOpen,
    );
  }

  @override
  Widget build(BuildContext context) {
    if (widget.items.length <= 1) {
      return Padding(
        padding: const EdgeInsets.fromLTRB(12, 6, 12, 6),
        child: widget.cardBuilder(widget.items.first),
      );
    }
    // 直接用「旧高度 → 新高度」一次性插值：展开时容器只往下长，下方内容被顺势推下去，
    // 不会先收一下再长回来。系统若开了「移除动画」，这里显式关掉，保证过渡仍然可见。
    final media = MediaQuery.of(context);
    return MediaQuery(
      data: media.copyWith(disableAnimations: false),
      child: AnimatedSize(
        duration: const Duration(milliseconds: 300),
        curve: Curves.easeOutCubic,
        alignment: Alignment.topCenter,
        clipBehavior: Clip.hardEdge,
        child: _buildContent(context),
      ),
    );
  }
}

/// 一叠里的一行摘要：点这一行直接进入对应的帖子/发布任务，底部按钮才展开全部。
class _UnfinishedRow extends StatelessWidget {
  const _UnfinishedRow({
    required this.item,
    required this.channelName,
    required this.onTap,
  });
  final PersonalPostItem item;
  final String channelName;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    final state = item.state;
    final accent = state == 'ACCEPTED';
    final failed = state == 'FAILED';
    final color = failed
        ? p.error
        : accent
        ? p.accent
        : p.secondary;
    final icon = failed
        ? Icons.error_outline_rounded
        : accent
        ? Icons.hourglass_bottom_rounded
        : item.isDraft || item.rateLimitedDraft
        ? Icons.edit_note_rounded
        : Icons.help_outline_rounded;
    return InkWell(
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(14, 9, 12, 9),
        child: Row(
          children: [
            Container(
              width: 32,
              height: 32,
              decoration: BoxDecoration(
                color: color.withValues(alpha: .12),
                borderRadius: BorderRadius.circular(8),
              ),
              child: Icon(icon, size: 18, color: color),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    item.title.isEmpty
                        ? hhText(context, '未命名草稿')
                        : item.title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w600,
                      height: 1.3,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    '$channelName · ${postDateLabel(context, item.sortAt)}',
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(fontSize: 11, color: p.secondary),
                  ),
                ],
              ),
            ),
            const SizedBox(width: 8),
            // 收起态只显示状态主词，括号里的细节留给展开后的卡片。
            ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 110),
              child: Text(
                hhPersonalPostStateText(context, state).split('·').first.trim(),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(fontSize: 11, color: color),
              ),
            ),
            Icon(Icons.chevron_right_rounded, size: 16, color: p.secondary),
          ],
        ),
      ),
    );
  }
}
