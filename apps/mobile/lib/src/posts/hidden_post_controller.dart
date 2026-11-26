part of 'post_controller.dart';

extension HiddenPostController on PostController {
  bool get supportsHiddenPosts => _api is HiddenPostApi;

  /// 帖子归属只作为“不感兴趣”的写入边界：调用能力接口，200 表示当前账号
  /// 发布过该帖（含本账号其他身份），404 表示不是自己的帖子，其余失败用
  /// null 表示无法核对，调用方应先停止写入而不是猜测。
  Future<bool?> _hiddenPostOwnedByAccount(
    _PostAuthority a,
    String id,
  ) async {
    try {
      await _accept(
        a,
        await _api.capabilities(sessionToken: a.token, postId: id),
      );
      return true;
    } catch (error) {
      if (error is _StalePostAuthority) return null;
      if (error is PostFailure && error.statusCode == 404) return false;
      if (error is PostFailure &&
          {
            PostFailureKind.unauthorized,
            PostFailureKind.unavailable,
          }.contains(error.kind)) {
        _failure(error, a);
      }
      return null;
    }
  }

  void _invalidateHiddenTimelineRead() {
    // An older in-flight page must not overwrite a newly committed preference.
    homeFeed.invalidatePendingRead(notify: false);
  }

  bool hiddenPostBusy(String id) => _hiddenWrites.contains(id);

  /// A returned version proves the requested state was committed or observed.
  /// A timeout is retried only with the original fence; a stale write cannot
  /// undo a newer intent from this or another device.
  Future<String?> changeHiddenPost(
    String id,
    bool desired, {
    String? version,
  }) async {
    if (!available || !supportsHiddenPosts || _hiddenWrites.contains(id)) {
      return null;
    }
    final pending = _hiddenUncertainVersions[id];
    if (pending != null && pending.desired != desired) {
      hiddenPostError = '上次操作的结果还在核对，请重试。';
      _notify();
      return null;
    }
    _hiddenWrites.add(id);
    hiddenPostError = null;
    _notify();
    final epoch = _epoch;
    _PostAuthority? authority;
    String? expected = pending?.version ?? version;
    final api = _api as HiddenPostApi;
    try {
      authority = await _authority();
      final a = authority;
      if (epoch != _epoch || !_current(a)) return null;
      if (desired) {
        final owned = await _hiddenPostOwnedByAccount(a, id);
        if (epoch != _epoch || !_current(a)) return null;
        if (owned != false) {
          // 自己的帖子不能被“不感兴趣”隐藏；归属无法核对时也先停下，
          // 避免把错误状态写到当前账号。
          hiddenPostError = owned == true
              ? '不能对自己发布的帖子设置不感兴趣。'
              : '暂时无法核对帖子归属，请重试。';
          return null;
        }
      }
      if (expected == null) {
        final read = await api.hiddenPost(sessionToken: a.token, postId: id);
        final state = await _accept(a, read);
        final observed = read.interactionVersion;
        if (observed == null) {
          throw const PostFailure(kind: PostFailureKind.invalidResponse);
        }
        if (state.hidden == desired) {
          _invalidateHiddenTimelineRead();
          return observed;
        }
        expected = observed;
      }
      _hiddenUncertainVersions[id] = (desired: desired, version: expected);
      final write = await api.setHiddenPost(
        sessionToken: a.token,
        postId: id,
        hidden: desired,
        version: expected,
      );
      final result = await _accept(a, write);
      final next = write.interactionVersion;
      if (next == null || next == expected || result.hidden != desired) {
        throw const PostFailure(kind: PostFailureKind.invalidResponse);
      }
      _hiddenUncertainVersions.remove(id);
      _invalidateHiddenTimelineRead();
      return next;
    } catch (error) {
      if (epoch != _epoch) return null;
      if (error is PostFailure &&
          {
            PostFailureKind.unauthorized,
            PostFailureKind.unavailable,
          }.contains(error.kind)) {
        _failure(error, authority);
      }
      final a = authority;
      if (a == null || !_current(a)) return null;
      if (pending == null &&
          error is PostFailure &&
          error.kind == PostFailureKind.rejected &&
          error.code != 'INTERACTION_VERSION_CONFLICT') {
        // This first request was definitively rejected. A rejection of a
        // retry cannot clear an older request whose outcome is still unknown.
        _hiddenUncertainVersions.remove(id);
        hiddenPostError = '暂时无法保存，请重试。';
        return null;
      }
      if (expected != null && _hiddenUncertainVersions.containsKey(id)) {
        try {
          final read = await api.hiddenPost(sessionToken: a.token, postId: id);
          final actual = await _accept(a, read);
          final observed = read.interactionVersion;
          if (observed == null) {
            throw const PostFailure(kind: PostFailureKind.invalidResponse);
          }
          if (observed != expected) {
            _hiddenUncertainVersions.remove(id);
            if (actual.hidden == desired) {
              _invalidateHiddenTimelineRead();
              return observed;
            }
            hiddenPostError = '状态已变化，请重新操作。';
            return null;
          }
        } catch (readError) {
          if (readError is PostFailure &&
              {
                PostFailureKind.unauthorized,
                PostFailureKind.unavailable,
              }.contains(readError.kind)) {
            _failure(readError, a);
          }
        }
      }
      if (_current(a)) {
        hiddenPostError = expected == null ? '暂时无法保存，请重试。' : '暂时无法确认结果，请重试。';
      }
      return null;
    } finally {
      if (epoch == _epoch) {
        _hiddenWrites.remove(id);
        _notify();
      }
    }
  }
}
